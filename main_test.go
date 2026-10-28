package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"testdata/good.txt"}, nil, &out, &errb); code != 0 {
		t.Fatalf("good: exit %d, %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "0 errors, 0 warnings") {
		t.Fatalf("good output: %s", out.String())
	}
	out.Reset()
	if code := run([]string{"testdata/bad.txt"}, nil, &out, &errb); code != 1 {
		t.Fatalf("bad: exit %d", code)
	}
	if code := run([]string{"--strict", "-"}, strings.NewReader("# A\n"), &out, &errb); code != 1 {
		t.Fatalf("strict with warning should fail, got %d", code)
	}
	if code := run([]string{"-"}, strings.NewReader("# A\n"), &out, &errb); code != 0 {
		t.Fatalf("warning only should pass, got %d", code)
	}
	if code := run(nil, nil, &out, &errb); code != 2 {
		t.Fatalf("no args: exit %d", code)
	}
}

func TestRunJSON(t *testing.T) {
	var out, errb bytes.Buffer
	run([]string{"--json", "testdata/good.txt"}, nil, &out, &errb)
	var v struct {
		Title string
		Links []struct{ URL string }
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Title != "FastWidget" || len(v.Links) != 4 {
		t.Fatalf("%+v", v)
	}
}
