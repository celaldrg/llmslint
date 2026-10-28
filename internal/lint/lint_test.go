package lint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func rules(r *Report) map[string]int {
	m := map[string]int{}
	for _, f := range r.Findings {
		m[f.Rule]++
	}
	return m
}

func TestGoodFileIsClean(t *testing.T) {
	b, err := os.ReadFile("../../testdata/good.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := Lint(string(b))
	if len(r.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v", r.Findings)
	}
	if r.Title != "FastWidget" || len(r.Sections) != 3 || len(r.Links) != 4 {
		t.Fatalf("bad parse: title=%q sections=%d links=%d", r.Title, len(r.Sections), len(r.Links))
	}
	if !strings.HasPrefix(r.Summary, "FastWidget is a Go library") {
		t.Fatalf("summary = %q", r.Summary)
	}
	if r.Links[0].Notes != "install and render your first widget" {
		t.Fatalf("notes = %q", r.Links[0].Notes)
	}
	if r.OptionalTokens == 0 || r.OptionalTokens >= r.ApproxTokens {
		t.Fatalf("optional tokens %d of %d", r.OptionalTokens, r.ApproxTokens)
	}
}

func TestBadFileFindsEveryProblem(t *testing.T) {
	b, err := os.ReadFile("../../testdata/bad.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := Lint(string(b))
	want := map[string]int{
		"h1-first": 1, "summary": 1, "absolute-url": 2, "prefer-markdown": 2,
		"duplicate-url": 1, "list-no-link": 1, "loose-text": 1, "optional-last": 1,
		"https": 1, "nested-heading": 1, "link-title": 1, "empty-section": 1,
	}
	got := rules(r)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("rule %s: got %d, want %d", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected rule %s", k)
		}
	}
	if r.Errors() != 2 {
		t.Errorf("errors = %d, want 2", r.Errors())
	}
}

func TestCases(t *testing.T) {
	cases := []struct {
		name, in, rule string
		sev            Severity
	}{
		{"empty file", "", "h1-required", Error},
		{"two titles", "# A\n> s\n# B\n", "single-h1", Error},
		{"broken link", "# A\n> s\n## Docs\n- [x](https://a.b\n", "link-format", Error},
		{"no host", "# A\n> s\n## Docs\n- [x](https:///path)\n", "link-url", Error},
		{"dash notes", "# A\n> s\n## Docs\n- [x](https://a.b/x.md) - notes\n", "notes-separator", Warning},
		{"mailto", "# A\n> s\n## Docs\n- [x](mailto:a@b.c)\n", "link-scheme", Warning},
		{"dup section", "# A\n> s\n## Docs\n- [x](https://a.b/1)\n## docs\n- [y](https://a.b/2)\n", "duplicate-section", Warning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Lint(c.in)
			for _, f := range r.Findings {
				if f.Rule == c.rule && f.Severity == c.sev {
					return
				}
			}
			t.Fatalf("missing %s %s in %+v", c.sev, c.rule, r.Findings)
		})
	}
}

func TestFencedCodeIsIgnored(t *testing.T) {
	in := "# A\n> s\n\n```\n# not a title\n## not a section\n```\n## Docs\n- [x](https://a.b/x.md)\n"
	r := Lint(in)
	if len(r.Findings) != 0 || len(r.Sections) != 1 {
		t.Fatalf("findings=%+v sections=%d", r.Findings, len(r.Sections))
	}
}

func TestCheckLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/ok.md":
		case "/head-hater.md":
			if req.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(w, req)
		}
	}))
	defer srv.Close()
	in := "# A\n> s\n## Docs\n- [ok](" + srv.URL + "/ok.md)\n- [gone](" + srv.URL + "/gone.md)\n- [head](" + srv.URL + "/head-hater.md)\n"
	r := Lint(in)
	CheckLinks(context.Background(), r, nil, srv.Client(), 2)
	dead := 0
	for _, f := range r.Findings {
		if f.Rule == "dead-link" {
			dead++
			if f.Line != 5 || !strings.Contains(f.Message, "404") {
				t.Errorf("unexpected dead link finding %+v", f)
			}
		}
	}
	if dead != 1 {
		t.Fatalf("dead links = %d, want 1: %+v", dead, r.Findings)
	}
}
