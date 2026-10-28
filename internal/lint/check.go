package lint

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CheckLinks requests every absolute http(s) link and adds a finding for
// each one that does not answer with a 2xx or 3xx status. base resolves
// relative links when it is non-nil.
func CheckLinks(ctx context.Context, r *Report, base *url.URL, client *http.Client, workers int) {
	if workers < 1 {
		workers = 8
	}
	type result struct {
		link Link
		msg  string
	}
	jobs := make(chan Link)
	out := make(chan result)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for l := range jobs {
				target, err := url.Parse(l.URL)
				if err != nil {
					continue
				}
				if !target.IsAbs() {
					if base == nil {
						continue
					}
					target = base.ResolveReference(target)
				}
				if target.Scheme != "http" && target.Scheme != "https" {
					continue
				}
				if msg := probe(ctx, client, target.String()); msg != "" {
					out <- result{l, msg}
				}
			}
		}()
	}
	go func() {
		for _, l := range r.Links {
			jobs <- l
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	for res := range out {
		r.Findings = append(r.Findings, Finding{res.link.Line, Error, "dead-link", fmt.Sprintf("%s: %s", res.link.URL, res.msg)})
	}
	sortFindings(r)
}

func probe(ctx context.Context, client *http.Client, u string) string {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		req, err := http.NewRequestWithContext(ctx, method, u, nil)
		if err != nil {
			cancel()
			return err.Error()
		}
		req.Header.Set("User-Agent", "llmslint (+https://github.com/celaldrg/llmslint)")
		resp, err := client.Do(req)
		cancel()
		if err != nil {
			return strings.TrimPrefix(err.Error(), method+" ")
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			return ""
		}
		// Some servers reject HEAD; retry once with GET.
		if method == http.MethodHead && (resp.StatusCode == 405 || resp.StatusCode == 403 || resp.StatusCode == 501) {
			continue
		}
		return resp.Status
	}
	return "request rejected"
}

func sortFindings(r *Report) {
	f := r.Findings
	for i := 1; i < len(f); i++ {
		for j := i; j > 0 && f[j].Line < f[j-1].Line; j-- {
			f[j], f[j-1] = f[j-1], f[j]
		}
	}
}
