// Command llmslint lints llms.txt files.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/celaldrg/llmslint/internal/lint"
)

var version = "0.1.0"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("llmslint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	links := fs.Bool("links", false, "also request every link and report dead ones")
	strict := fs.Bool("strict", false, "exit 1 on warnings too")
	quiet := fs.Bool("q", false, "only print findings, no summary")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: llmslint [flags] <file | url | domain | ->")
		fmt.Fprintln(stderr, "  a bare domain such as example.com fetches https://example.com/llms.txt")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "llmslint", version)
		return 0
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	client := &http.Client{Timeout: 20 * time.Second}
	text, base, err := load(fs.Arg(0), stdin, client)
	if err != nil {
		fmt.Fprintln(stderr, "llmslint:", err)
		return 2
	}
	rep := lint.Lint(text)
	if *links {
		lint.CheckLinks(context.Background(), rep, base, client, 8)
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	} else {
		printText(stdout, fs.Arg(0), rep, *quiet)
	}
	if rep.Errors() > 0 || (*strict && rep.Warnings() > 0) {
		return 1
	}
	return 0
}

func load(arg string, stdin io.Reader, client *http.Client) (string, *url.URL, error) {
	if arg == "-" {
		b, err := io.ReadAll(stdin)
		return string(b), nil, err
	}
	if _, err := os.Stat(arg); err == nil {
		b, err := os.ReadFile(arg)
		return string(b), nil, err
	}
	target := arg
	if !strings.Contains(target, "://") {
		if !strings.Contains(target, ".") {
			return "", nil, fmt.Errorf("%s: no such file", arg)
		}
		target = "https://" + strings.TrimSuffix(target, "/")
		if !strings.HasSuffix(target, ".txt") {
			target += "/llms.txt"
		}
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", nil, err
	}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(b), u, err
}

func printText(w io.Writer, name string, r *lint.Report, quiet bool) {
	for _, f := range r.Findings {
		fmt.Fprintf(w, "%s:%d: %s [%s] %s\n", name, f.Line, f.Severity, f.Rule, f.Message)
	}
	if quiet {
		return
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(w)
	}
	title := r.Title
	if title == "" {
		title = "(no title)"
	}
	fmt.Fprintf(w, "%s: %d sections, %d links, ~%d tokens", title, len(r.Sections), len(r.Links), r.ApproxTokens)
	if r.OptionalTokens > 0 {
		fmt.Fprintf(w, " (~%d in Optional)", r.OptionalTokens)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d errors, %d warnings\n", r.Errors(), r.Warnings())
}
