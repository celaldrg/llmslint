// Package lint checks llms.txt files against the format described at
// https://llmstxt.org: an H1 title, an optional blockquote summary, optional
// free-form notes, then H2 sections that hold Markdown link lists.
package lint

import (
	"bufio"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Severity of a finding.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Finding is one problem found in a file.
type Finding struct {
	Line     int      `json:"line"`
	Severity Severity `json:"severity"`
	Rule     string   `json:"rule"`
	Message  string   `json:"message"`
}

// Link is one entry of a file list.
type Link struct {
	Line    int    `json:"line"`
	Section string `json:"section"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Notes   string `json:"notes,omitempty"`
}

// Section is an H2 block.
type Section struct {
	Name     string `json:"name"`
	Line     int    `json:"line"`
	Links    int    `json:"links"`
	Optional bool   `json:"optional"`
}

// Report is the result of linting one document.
type Report struct {
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	Sections       []Section `json:"sections"`
	Links          []Link    `json:"links"`
	Findings       []Finding `json:"findings"`
	ApproxTokens   int       `json:"approx_tokens"`
	OptionalTokens int       `json:"optional_tokens"`
}

// Errors returns the number of error findings.
func (r *Report) Errors() int { return r.count(Error) }

// Warnings returns the number of warning findings.
func (r *Report) Warnings() int { return r.count(Warning) }

func (r *Report) count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

var (
	listItemRe = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	linkItemRe = regexp.MustCompile(`^\[([^\]]*)\]\(([^)\s]*)\)\s*(?:([:\-–—])\s*(.*))?$`)
	headingRe  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	fenceRe    = regexp.MustCompile("^\\s*(```|~~~)")
)

// Lint parses text and reports problems. Line numbers are 1-based.
func Lint(text string) *Report {
	r := &Report{Sections: []Section{}, Links: []Link{}, Findings: []Finding{}}
	add := func(line int, s Severity, rule, format string, a ...any) {
		r.Findings = append(r.Findings, Finding{line, s, rule, fmt.Sprintf(format, a...)})
	}

	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var (
		lineNo      int
		sawContent  bool
		h1Line      int
		inFence     bool
		cur         *Section
		seenURL     = map[string]int{}
		seenSection = map[string]int{}
		summaryDone bool
		optChars    int
	)
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if cur != nil && cur.Optional {
			optChars += len(line) + 1
		}
		if fenceRe.MatchString(line) {
			inFence = !inFence
			sawContent = true
			continue
		}
		if inFence || trimmed == "" {
			continue
		}

		if m := headingRe.FindStringSubmatch(trimmed); m != nil {
			level, name := len(m[1]), m[2]
			switch {
			case level == 1:
				if h1Line != 0 {
					add(lineNo, Error, "single-h1", "second H1 heading (first one is on line %d); llms.txt has exactly one title", h1Line)
				} else {
					if sawContent {
						add(lineNo, Error, "h1-first", "the H1 title must be the first thing in the file")
					}
					h1Line = lineNo
					r.Title = name
					if name == "" {
						add(lineNo, Error, "h1-empty", "the H1 title is empty")
					}
				}
			case level == 2:
				if h1Line == 0 {
					add(lineNo, Error, "h1-first", "H2 section before the H1 title")
				}
				if prev, ok := seenSection[strings.ToLower(name)]; ok {
					add(lineNo, Warning, "duplicate-section", "section %q already appears on line %d", name, prev)
				}
				seenSection[strings.ToLower(name)] = lineNo
				if cur != nil && cur.Optional {
					optChars -= len(line) + 1
				}
				closeSection(r, cur, add)
				r.Sections = append(r.Sections, Section{Name: name, Line: lineNo, Optional: strings.EqualFold(name, "optional")})
				cur = &r.Sections[len(r.Sections)-1]
				if cur.Optional {
					optChars += len(line) + 1
				}
			default:
				if cur != nil {
					add(lineNo, Warning, "nested-heading", "H%d inside a file-list section; tools that split on H2 will treat it as list content", level)
				}
			}
			sawContent = true
			continue
		}
		sawContent = true

		if cur == nil {
			if strings.HasPrefix(trimmed, ">") {
				if summaryDone {
					continue
				}
				r.Summary = strings.TrimSpace(strings.TrimSpace(r.Summary + " " + strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))))
				continue
			}
			if h1Line != 0 {
				summaryDone = true
			}
			continue
		}

		m := listItemRe.FindStringSubmatch(line)
		if m == nil {
			add(lineNo, Warning, "loose-text", "text inside a section that is not a list item; file-list sections should only hold \"- [name](url): notes\" lines")
			continue
		}
		lm := linkItemRe.FindStringSubmatch(strings.TrimSpace(m[1]))
		if lm == nil {
			if strings.HasPrefix(strings.TrimSpace(m[1]), "[") {
				add(lineNo, Error, "link-format", "list item is not \"[name](url)\" with optional \": notes\"")
			} else {
				add(lineNo, Warning, "list-no-link", "list item has no link; file-list sections are meant to point at documents")
			}
			continue
		}
		title, u, notes := strings.TrimSpace(lm[1]), lm[2], strings.TrimSpace(lm[4])
		if lm[3] != "" && lm[3] != ":" {
			add(lineNo, Warning, "notes-separator", "notes are separated with %q; the spec uses \": \"", lm[3])
		}
		cur.Links++
		r.Links = append(r.Links, Link{lineNo, cur.Name, title, u, notes})
		if title == "" {
			add(lineNo, Error, "link-title", "link has no name")
		}
		checkURL(lineNo, u, add)
		if prev, ok := seenURL[u]; ok && u != "" {
			add(lineNo, Warning, "duplicate-url", "%s is already listed on line %d", u, prev)
		} else {
			seenURL[u] = lineNo
		}
		if strings.HasSuffix(strings.SplitN(u, "#", 2)[0], ".html") {
			add(lineNo, Warning, "prefer-markdown", "link points to HTML; the spec suggests serving a clean .md version (e.g. %s.md)", u)
		}
	}
	closeSection(r, cur, add)

	if h1Line == 0 {
		add(1, Error, "h1-required", "no H1 title; it is the only required part of llms.txt")
	}
	if h1Line != 0 && r.Summary == "" {
		add(h1Line, Warning, "summary", "no blockquote summary (\"> ...\") after the title; it is the first thing a model reads")
	}
	if len(r.Sections) > 0 && r.Sections[len(r.Sections)-1].Optional == false {
		for _, s := range r.Sections {
			if s.Optional {
				add(s.Line, Warning, "optional-last", "the \"Optional\" section should come last so it can be cut from short contexts")
			}
		}
	}

	r.ApproxTokens = (len(text) + 3) / 4
	r.OptionalTokens = (optChars + 3) / 4
	sort.SliceStable(r.Findings, func(i, j int) bool { return r.Findings[i].Line < r.Findings[j].Line })
	return r
}

func closeSection(r *Report, s *Section, add func(int, Severity, string, string, ...any)) {
	if s != nil && s.Links == 0 {
		add(s.Line, Warning, "empty-section", "section %q has no links", s.Name)
	}
}

func checkURL(line int, raw string, add func(int, Severity, string, string, ...any)) {
	if raw == "" {
		add(line, Error, "link-url", "link has an empty URL")
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		add(line, Error, "link-url", "cannot parse URL %q: %v", raw, err)
		return
	}
	switch {
	case u.Scheme == "http":
		add(line, Warning, "https", "%s uses plain http", raw)
	case u.Scheme == "https":
	case u.Scheme == "":
		add(line, Warning, "absolute-url", "%s is relative; agents often fetch llms.txt out of context, so absolute URLs are safer", raw)
	default:
		add(line, Warning, "link-scheme", "%s uses the %q scheme", raw, u.Scheme)
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host == "" {
		add(line, Error, "link-url", "%s has no host", raw)
	}
}
