# llmslint

[![CI](https://github.com/celaldrg/llmslint/actions/workflows/ci.yml/badge.svg)](https://github.com/celaldrg/llmslint/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/celaldrg/llmslint.svg)](https://pkg.go.dev/github.com/celaldrg/llmslint)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**A linter for `llms.txt`, the file that tells AI agents where your docs are.**

More and more sites publish an [`/llms.txt`](https://llmstxt.org): a Markdown file with a title, a one-paragraph summary and lists of links to clean Markdown pages, so coding agents and chat assistants can read the docs without scraping HTML. The format is small, but it is easy to get subtly wrong: a stray paragraph before the title, relative links that break once an agent fetches the file out of context, an `Optional` section that is not last, links that quietly 404.

`llmslint` checks a file, a URL or a whole domain in one command, tells you what to fix with line numbers, and can fail CI when your `llms.txt` drifts. One static binary, no dependencies.

## Install

```sh
go install github.com/celaldrg/llmslint@latest
```

Or grab a binary from the [releases page](https://github.com/celaldrg/llmslint/releases).

## Usage

```sh
llmslint llms.txt              # a local file
llmslint docs.example.com      # fetches https://docs.example.com/llms.txt
llmslint https://example.com/llms-full.txt
cat llms.txt | llmslint -      # stdin
llmslint --links llms.txt      # also request every link and report dead ones
llmslint --json llms.txt       # machine-readable report
llmslint --strict llms.txt     # warnings fail too (exit 1)
```

Exit codes: `0` clean (or warnings only), `1` errors found (or warnings with `--strict`), `2` the input could not be read.

## Example

A file with most of the usual mistakes ([`testdata/bad.txt`](testdata/bad.txt)):

```text
$ llmslint testdata/bad.txt
testdata/bad.txt:2: error [h1-first] the H1 title must be the first thing in the file
testdata/bad.txt:2: warning [summary] no blockquote summary ("> ...") after the title; it is the first thing a model reads
testdata/bad.txt:6: warning [absolute-url] /install.html is relative; agents often fetch llms.txt out of context, so absolute URLs are safer
testdata/bad.txt:6: warning [prefer-markdown] link points to HTML; the spec suggests serving a clean .md version (e.g. /install.html.md)
testdata/bad.txt:7: warning [absolute-url] /install.html is relative; agents often fetch llms.txt out of context, so absolute URLs are safer
testdata/bad.txt:7: warning [duplicate-url] /install.html is already listed on line 6
testdata/bad.txt:7: warning [prefer-markdown] link points to HTML; the spec suggests serving a clean .md version (e.g. /install.html.md)
testdata/bad.txt:8: warning [list-no-link] list item has no link; file-list sections are meant to point at documents
testdata/bad.txt:9: warning [loose-text] text inside a section that is not a list item; file-list sections should only hold "- [name](url): notes" lines
testdata/bad.txt:11: warning [optional-last] the "Optional" section should come last so it can be cut from short contexts
testdata/bad.txt:13: warning [https] http://acme.dev/blog.md uses plain http
testdata/bad.txt:17: warning [nested-heading] H3 inside a file-list section; tools that split on H2 will treat it as list content
testdata/bad.txt:18: error [link-title] link has no name
testdata/bad.txt:19: warning [empty-section] section "Empty" has no links

Acme: 4 sections, 4 links, ~69 tokens (~12 in Optional)
2 errors, 12 warnings
```

The spec's own file:

```text
$ llmslint llmstxt.org
llms.txt: 1 sections, 3 links, ~160 tokens
0 errors, 0 warnings
```

The token count is a rough size estimate (characters / 4), with the share inside `## Optional` shown separately, since that is the part a tool may drop for a short context window.

## Rules

| Rule | Level | What it catches |
| --- | --- | --- |
| `h1-required` | error | No `# Title`. It is the only required part of the format. |
| `h1-first` | error | Anything other than the title comes first, or a section appears before it. |
| `single-h1` | error | More than one `# Title`. |
| `link-format` | error | A list item that starts like a link but is not `[name](url)`. |
| `link-title` | error | `[](url)` with no name. |
| `link-url` | error | Empty URL, unparsable URL, or `https://` with no host. |
| `dead-link` | error | With `--links`: the URL does not answer 2xx/3xx (HEAD, then GET for servers that refuse HEAD). |
| `summary` | warning | No `> blockquote` summary after the title. |
| `absolute-url` | warning | Relative links; agents often fetch `llms.txt` on its own. |
| `https` / `link-scheme` | warning | Plain `http`, or schemes like `mailto:`. |
| `duplicate-url` | warning | The same URL listed twice. |
| `duplicate-section` | warning | Two `##` sections with the same name. |
| `empty-section` | warning | A `##` section with no links. |
| `optional-last` | warning | `## Optional` is not the final section. |
| `prefer-markdown` | warning | Links to `.html` where a `.md` version is recommended. |
| `list-no-link` | warning | A list item in a section with no link at all. |
| `loose-text` | warning | Paragraphs inside a file-list section. |
| `nested-heading` | warning | `###` headings inside a section; parsers that split on `##` treat them as list content. |
| `notes-separator` | warning | `[name](url) - notes` instead of the spec's `[name](url): notes`. |

Fenced code blocks are skipped, so example headings inside them do not count.

## In CI

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: stable
- run: go install github.com/celaldrg/llmslint@latest
- run: llmslint --links public/llms.txt
```

## Library

The checker lives in `internal/lint` for now; the JSON output (`--json`) is the stable interface for other tools. It includes the title, summary, sections, every link with its line and notes, the findings and the size estimate.

## Credits

The format is the [llms.txt proposal](https://llmstxt.org) by Jeremy Howard / Answer.AI. This is an independent implementation written from the public spec.

## License

[MIT](LICENSE)
