package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	trn "github.com/aadjm/tabular-recipe-notation-renderer"
)

// The fixtures the other cards' goldens use. The CLI must render the very same
// bytes the library does, so tests compare against library output and, for the
// standalone page, against the committed browser-verified golden.
const (
	chaiFixture = "../../examples/input/chai.trn.md"
	chaiDoc     = "../../examples/output/chai.doc.html"
)

// update mirrors the engine package's -update flag (golden_test.go) so that
// `go test ./... -update` — the command SPEC-ENGINE §6 names — does not fail
// here with "flag provided but not defined". This package owns no goldens of
// its own: it compares against the library's output, and the one committed file
// it reads (chai.doc.html) is rewritten by the engine package. Rewriting it from
// here too would race that rewrite, because `go test ./...` runs the two test
// binaries in parallel.
var update = flag.Bool("update", false, "accepted for `go test ./... -update`; the engine package owns the goldens")

// result is one invocation of run.
type result struct {
	code   int
	stdout string
	stderr string
}

// runCLI drives run exactly as main does, with stdin as the given string.
func runCLI(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

// mustRead returns a committed test file as a string.
func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// parseFixture parses a fixture through the library, independently of the CLI.
func parseFixture(t *testing.T, path string) *trn.Recipe {
	t.Helper()
	r, _ := trn.Parse(mustRead(t, path))
	return r
}

// TestNoFlagWritesBothFragments covers the default invocation: the two view
// fragments, in §4 order, separated by a newline (SPEC-ENGINE §5).
func TestNoFlagWritesBothFragments(t *testing.T) {
	got := runCLI(t, mustRead(t, chaiFixture))
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, exitOK, got.stderr)
	}
	want := string(trn.RenderTable(parseFixture(t, chaiFixture))) + "\n" +
		string(trn.RenderFlow(parseFixture(t, chaiFixture)))
	if got.stdout != want {
		t.Errorf("default output is not RenderTable + newline + RenderFlow")
	}
	if got.stderr != "" {
		t.Errorf("clean document wrote to stderr: %q", got.stderr)
	}
}

// TestViewSelectsFragment checks each --view value against the library call it
// names, and that a single view really is alone (no other view's markup).
func TestViewSelectsFragment(t *testing.T) {
	src := mustRead(t, chaiFixture)
	r := parseFixture(t, chaiFixture)
	cases := []struct {
		view  string
		want  string
		other string // a marker the other view has and this one must not
	}{
		{viewTable, string(trn.RenderTable(r)), `class="trn-flow-step`},
		{viewFlow, string(trn.RenderFlow(r)), `class="trn-table"`},
		{viewBoth, string(trn.RenderTable(r)) + "\n" + string(trn.RenderFlow(r)), ""},
	}
	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			got := runCLI(t, src, "--view", tc.view)
			if got.code != exitOK {
				t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, exitOK, got.stderr)
			}
			if got.stdout != tc.want {
				t.Errorf("--view %s output differs from the library's fragment", tc.view)
			}
			if tc.other != "" && strings.Contains(got.stdout, tc.other) {
				t.Errorf("--view %s output contains the other view (%s)", tc.view, tc.other)
			}
		})
	}
}

// TestFlagsMayFollowTheFile is the card's own invocation shape:
// `trn-render fixture.md --view table --standalone`. Go's flag package stops at
// the first positional, so the CLI parses its own line and must accept both
// orders.
func TestFlagsMayFollowTheFile(t *testing.T) {
	before := runCLI(t, "", "--view", "table", chaiFixture)
	after := runCLI(t, "", chaiFixture, "--view", "table")
	if after.code != exitOK || before.code != exitOK {
		t.Fatalf("exit = %d and %d, want %d", before.code, after.code, exitOK)
	}
	if after.stdout != before.stdout {
		t.Error("flag position changed the output")
	}
	if after.stdout != string(trn.RenderTable(parseFixture(t, chaiFixture))) {
		t.Error("table fragment differs from RenderTable")
	}
}

// TestStandaloneMatchesCommittedPage ties the CLI to the artifact that was
// browser-verified in TRN-ENG-5: for the canonical chai fixture, --standalone
// must reproduce testdata/chai.doc.html byte for byte.
func TestStandaloneMatchesCommittedPage(t *testing.T) {
	if *update {
		t.Skip("chai.doc.html is being rewritten by the engine package's -update run")
	}
	got := runCLI(t, "", "--standalone", chaiFixture)
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, exitOK, got.stderr)
	}
	if want := mustRead(t, chaiDoc); got.stdout != want {
		t.Errorf("--standalone output differs from %s", chaiDoc)
	}
	if !strings.HasPrefix(got.stdout, "<!doctype html>") || !strings.HasSuffix(got.stdout, "</html>\n") {
		t.Error("standalone output is not a complete HTML document")
	}
}

// TestStandaloneTitleOverride checks --title reaches both the <title> element
// and the page heading, escaped as text, and that it wins over the parsed title.
func TestStandaloneTitleOverride(t *testing.T) {
	const title = `Chai & <Friends> "Extra"`
	got := runCLI(t, mustRead(t, chaiFixture), "--standalone", "--title", title)
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, exitOK, got.stderr)
	}
	if strings.Contains(got.stdout, "<Friends>") {
		t.Error("title was not escaped")
	}
	if !strings.Contains(got.stdout, `<title>Chai &amp; &lt;Friends&gt; &#34;Extra&#34;</title>`) {
		t.Errorf("<title> does not carry the override; document starts:\n%s", firstLines(got.stdout, 12))
	}
	if !strings.Contains(got.stdout, `<h1 class="trn-doc-title">Chai &amp; &lt;Friends&gt; &#34;Extra&#34;</h1>`) {
		t.Error("<h1> does not carry the override")
	}
	if strings.Contains(got.stdout, "Masala Chai") {
		t.Error("the overridden title still appears in the page")
	}
	// Same bytes the library writes when the field is set, so the CLI adds
	// nothing of its own to the page.
	r := parseFixture(t, chaiFixture)
	r.Title = title
	if got.stdout != string(trn.RenderDocument(r)) {
		t.Error("--title output differs from RenderDocument with Recipe.Title set")
	}
}

// TestStandaloneViewSelection checks that --standalone respects --view so each
// view can be rendered separately as a standalone document or together.
func TestStandaloneViewSelection(t *testing.T) {
	src := mustRead(t, chaiFixture)
	r := parseFixture(t, chaiFixture)

	both := runCLI(t, src, "--standalone")
	if both.code != exitOK {
		t.Fatalf("exit = %d, want %d", both.code, exitOK)
	}
	if !strings.Contains(both.stdout, `<table class="trn-table">`) || !strings.Contains(both.stdout, `<figure class="trn-flow">`) {
		t.Error("default standalone page is missing one of the two views")
	}

	tableOnly := runCLI(t, src, "--standalone", "--view", viewTable)
	if tableOnly.code != exitOK {
		t.Fatalf("table exit = %d, want %d", tableOnly.code, exitOK)
	}
	if tableOnly.stdout != string(trn.RenderDocumentView(r, trn.ViewTable)) {
		t.Error("--standalone --view table differs from RenderDocumentView(ViewTable)")
	}
	if !strings.Contains(tableOnly.stdout, `<table class="trn-table">`) || strings.Contains(tableOnly.stdout, `<figure class="trn-flow">`) {
		t.Error("--standalone --view table did not isolate table view")
	}

	flowOnly := runCLI(t, src, "--standalone", "--view", viewFlow)
	if flowOnly.code != exitOK {
		t.Fatalf("flow exit = %d, want %d", flowOnly.code, exitOK)
	}
	if flowOnly.stdout != string(trn.RenderDocumentView(r, trn.ViewFlow)) {
		t.Error("--standalone --view flow differs from RenderDocumentView(ViewFlow)")
	}
	if !strings.Contains(flowOnly.stdout, `<figure class="trn-flow">`) || strings.Contains(flowOnly.stdout, `<table class="trn-table">`) {
		t.Error("--standalone --view flow did not isolate flow view")
	}

	for _, bad := range []string{"<input", "<style", "<script", "trn-view-toggle", "<h2"} {
		if strings.Contains(both.stdout, bad) {
			t.Errorf("standalone page contains %q; the engine emits structure only", bad)
		}
	}
}

// TestStdinAndDash checks both stdin spellings produce the bytes the file
// argument produces.
func TestStdinAndDash(t *testing.T) {
	src := mustRead(t, chaiFixture)
	fromFile := runCLI(t, "", chaiFixture)
	noArg := runCLI(t, src)
	dash := runCLI(t, src, "-")
	for name, got := range map[string]result{"no argument": noArg, "-": dash} {
		if got.code != exitOK {
			t.Fatalf("%s: exit = %d, want %d (stderr: %s)", name, got.code, exitOK, got.stderr)
		}
		if got.stdout != fromFile.stdout {
			t.Errorf("%s: stdin output differs from file output", name)
		}
	}
}

// TestWarningsExitZero checks SPEC-ENGINE §5's exit-code rule: a document with
// warnings renders, exits 0, and reports every code on stderr.
func TestWarningsExitZero(t *testing.T) {
	// No title (W6), a repeated step number (W1), an empty verb (W7) and an
	// ingredient that never reaches the final step (W4).
	const src = "[1] Mix: sugar\n[1] : salt\n"
	got := runCLI(t, src, "--standalone")
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d for a document with warnings", got.code, exitOK)
	}
	if got.stdout == "" {
		t.Fatal("no HTML was written")
	}
	for _, code := range []string{"W1", "W4", "W6", "W7"} {
		if !strings.Contains(got.stderr, "trn-render: "+code+": ") {
			t.Errorf("stderr does not report %s:\n%s", code, got.stderr)
		}
	}
	// Warnings are annotations, not a degraded render: the page is the library's
	// page for the same document, byte for byte.
	r, _ := trn.Parse(src)
	if want := string(trn.RenderDocument(r)); got.stdout != want {
		t.Error("warnings changed the rendered page")
	}
}

// TestWarningCodesAreOrdered checks the merged W1 … W7 list comes out in code
// order, which is the order §3 promises for the parser's half plus the
// render-time W4 the CLI adds.
func TestWarningCodesAreOrdered(t *testing.T) {
	const src = "[2] Mix: sugar\n[2] Fold: salt, #9\n"
	got := runCLI(t, src)
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d", got.code, exitOK)
	}
	last := ""
	for _, line := range strings.Split(strings.TrimSpace(got.stderr), "\n") {
		code, _, ok := strings.Cut(strings.TrimPrefix(line, "trn-render: "), ": ")
		if !ok {
			t.Fatalf("unexpected stderr line %q", line)
		}
		if code < last {
			t.Errorf("warning %s printed after %s: out of code order\n%s", code, last, got.stderr)
		}
		last = code
	}
	if last == "" {
		t.Fatal("no warnings were reported")
	}
	// W2 (reference to a missing step) and W4 (ingredient never reaching the
	// final step) are both expected here; W1 too (numbers do not start at 1).
	for _, code := range []string{"W1", "W2", "W4", "W6"} {
		if !strings.Contains(got.stderr, code+": ") {
			t.Errorf("missing %s in:\n%s", code, got.stderr)
		}
	}
}

// TestWarningsAlsoReportedForFragments documents the one place the CLI is
// broader than §5's sentence: warnings go to stderr in fragment mode too, where
// stderr is free and the operator still wants to know. stdout stays clean.
func TestWarningsAlsoReportedForFragments(t *testing.T) {
	const src = "[1] Mix: sugar\n[1] : salt\n"
	got := runCLI(t, src, "--view", "table")
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d", got.code, exitOK)
	}
	if !strings.HasPrefix(got.stdout, `<table class="trn-table">`) {
		t.Errorf("stdout is not the table fragment: %s", firstLines(got.stdout, 2))
	}
	if !strings.Contains(got.stderr, "W1: ") {
		t.Errorf("warnings missing from stderr: %q", got.stderr)
	}
}

// TestMissingFileIsIOFailure checks the only non-zero content exit: the input
// could not be read. Nothing is written to stdout.
func TestMissingFileIsIOFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.trn.md")
	got := runCLI(t, "", missing)
	if got.code != exitIO {
		t.Fatalf("exit = %d, want %d for a missing file", got.code, exitIO)
	}
	if got.stdout != "" {
		t.Errorf("stdout is not empty: %q", got.stdout)
	}
	if !strings.Contains(got.stderr, missing) {
		t.Errorf("stderr does not name the file: %q", got.stderr)
	}
}

// TestUnreadableInputIsIOFailure covers the other I/O failure: a directory where
// a file is expected.
func TestUnreadableInputIsIOFailure(t *testing.T) {
	got := runCLI(t, "", t.TempDir())
	if got.code != exitIO {
		t.Fatalf("exit = %d, want %d for a directory", got.code, exitIO)
	}
	if got.stdout != "" {
		t.Errorf("stdout is not empty: %q", got.stdout)
	}
}

// failingWriter fails every write, standing in for a full disk or a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// TestOutputFailureIsIOFailure checks a write error is reported rather than
// swallowed.
func TestOutputFailureIsIOFailure(t *testing.T) {
	var errOut bytes.Buffer
	code := run([]string{"-"}, strings.NewReader("T\n[1] Mix: sugar\n"), failingWriter{}, &errOut)
	if code != exitIO {
		t.Fatalf("exit = %d, want %d for a failed write", code, exitIO)
	}
	if !strings.Contains(errOut.String(), "writing output") {
		t.Errorf("stderr does not explain the failure: %q", errOut.String())
	}
}

// TestUsageErrors checks every malformed command line exits 2, explains itself
// on stderr and writes no HTML.
func TestUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"unknown flag":        {"--verbose"},
		"bad view":            {"--view", "matrix"},
		"empty view":          {"--view="},
		"view without value":  {"--view"},
		"title without value": {"--title"},
		"empty title":         {"--title="},
		"standalone value":    {"--standalone=yes"},
		"two files":           {"a.md", "b.md"},
		"empty file name":     {""},
		"help value":          {"--help=1"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			got := runCLI(t, "T\n[1] Mix: sugar\n", args...)
			if got.code != exitUsage {
				t.Fatalf("exit = %d, want %d", got.code, exitUsage)
			}
			if got.stdout != "" {
				t.Errorf("stdout is not empty: %q", got.stdout)
			}
			if !strings.HasPrefix(got.stderr, "trn-render: ") || !strings.Contains(got.stderr, "usage: trn-render") {
				t.Errorf("stderr does not explain the usage error: %q", got.stderr)
			}
		})
	}
}

// TestHelp checks -h and --help print the usage on stdout and exit 0, without
// reading stdin.
func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		got := runCLI(t, "T\n[1] Mix: sugar\n", flag)
		if got.code != exitOK {
			t.Errorf("%s: exit = %d, want %d", flag, got.code, exitOK)
		}
		if !strings.HasPrefix(got.stdout, "usage: trn-render") {
			t.Errorf("%s: stdout is not the usage: %q", flag, got.stdout)
		}
		if got.stderr != "" {
			t.Errorf("%s: wrote to stderr: %q", flag, got.stderr)
		}
	}
}

// TestDoubleDashEndsFlags checks "--" lets a file whose name starts with a dash
// be read.
func TestDoubleDashEndsFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-recipe.trn.md"), []byte("T\n[1] Mix: sugar\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Chdir(dir)
	got := runCLI(t, "", "--", "-recipe.trn.md")
	if got.code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", got.code, exitOK, got.stderr)
	}
	if !strings.Contains(got.stdout, "<table") {
		t.Error("the file after -- was not rendered")
	}
}

// TestRunsAreIndependent is the "no global state" check: the same process runs a
// document with warnings and then a clean one, and the second invocation must
// not inherit the first's warnings or its title.
func TestRunsAreIndependent(t *testing.T) {
	dirty := runCLI(t, "[1] Mix: sugar\n[2] #1\n")
	clean := runCLI(t, mustRead(t, chaiFixture), "--standalone")
	if dirty.stderr == "" {
		t.Fatal("the dirty document produced no warnings")
	}
	if clean.stderr != "" {
		t.Errorf("warnings leaked into the second run: %q", clean.stderr)
	}
	if !strings.Contains(clean.stdout, "Masala Chai") || strings.Contains(clean.stdout, "Recipe") {
		t.Error("the second run did not use its own recipe's title")
	}
}

// firstLines returns up to n leading lines of s, for error messages.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
