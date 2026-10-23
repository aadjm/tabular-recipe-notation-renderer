package trn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bannedInOutput is the pure-HTML contract of SPEC-ENGINE §4.3, one token per
// thing the engine must never emit: no stylesheet, no script, no form control,
// no CSS custom property or media query, no embedded or referenced image, no
// external resource, no event handler.
var bannedInOutput = []string{
	"<style", "</style", "<script", "</script", "<noscript", "<input",
	"<label", "<link", "<svg", "<canvas", "<img", "<iframe", "<object",
	"<embed", "<video", "<audio",
	"style=", "--trn-", "@media", "@import", "@font-face", "url(",
	"src=", "href=", "javascript:", "http://", "https://", "//cdn",
	"onload=", "onclick=", "onerror=", "onchange=", "onsubmit=",
}

// TestRenderOutputIsPureHTML is acceptance criterion 1: every renderer, for
// every fixture, emits classed semantic HTML and nothing else. It scans the
// four call shapes the engine offers — each view alone, both views in the
// standalone page, and the degenerate empty recipe — so no code path can
// smuggle styling, script, a control or an external reference into the output.
func TestRenderOutputIsPureHTML(t *testing.T) {
	recipes := map[string]*Recipe{"nil": nil, "empty": {}}
	for _, name := range fixtureNames(t) {
		recipes[name] = loadRecipe(t, name)
	}

	for name, r := range recipes {
		for view, out := range map[string]string{
			"table":    string(RenderTable(r)),
			"flow":     string(RenderFlow(r)),
			"document": string(RenderDocument(r)),
		} {
			for _, bad := range bannedInOutput {
				if strings.Contains(out, bad) {
					t.Errorf("%s/%s: output contains %q; the engine emits structure only (SPEC-ENGINE §4.3)", name, view, bad)
				}
			}
		}
	}
}

// TestDocumentStructure checks the structural half of §4.3 on the standalone
// page: a language, one h1 carrying the title, a logical heading order, a
// labelled section per view, a real table with scoped headers, and both views
// present in order — with no switcher, because styling and controls are the
// site's job (SPEC-SITE §7).
func TestDocumentStructure(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		doc := string(RenderDocument(r))

		if !strings.HasPrefix(doc, "<!doctype html>\n") {
			t.Errorf("%s: document does not start with a doctype", name)
		}
		if !strings.Contains(doc, `<html lang="`+defaultLang+`">`) {
			t.Errorf("%s: missing lang attribute on <html>", name)
		}
		if !strings.Contains(doc, "<title>"+htmlText(t, r.Title)+"</title>") {
			t.Errorf("%s: <title> does not carry the recipe title", name)
		}
		if n := strings.Count(doc, "<h1"); n != 1 {
			t.Errorf("%s: h1 count = %d, want exactly 1", name, n)
		}
		if n := strings.Count(doc, "<h2"); n != 0 {
			t.Errorf("%s: h2 count = %d, want 0 (no section headers)", name, n)
		}
		if n := strings.Count(doc, "<h1"); n > 0 && !strings.Contains(doc, `<h1 class="trn-doc-title">`+htmlText(t, r.Title)+`</h1>`) {
			t.Errorf("%s: page heading does not carry the recipe title", name)
		}

		// Both views, in order, each in its own section.
		tableAt := strings.Index(doc, `<table class="trn-table">`)
		flowAt := strings.Index(doc, `<figure class="trn-flow">`)
		if tableAt < 0 || flowAt < 0 {
			t.Errorf("%s: document is missing a view (table at %d, flow at %d)", name, tableAt, flowAt)
		}
		if tableAt > flowAt {
			t.Errorf("%s: the tabular view must come first", name)
		}
		for _, want := range []string{
			`<section class="trn-doc-view trn-doc-view-table">`,
			`<section class="trn-doc-view trn-doc-view-flow">`,
			`<th scope="row"`,
		} {
			if !strings.Contains(doc, want) {
				t.Errorf("%s: document is missing %q", name, want)
			}
		}

		// Verify toggleable views via RenderDocumentView and RenderView.
		docTable := string(RenderDocumentView(r, ViewTable))
		if !strings.Contains(docTable, `<table class="trn-table">`) || strings.Contains(docTable, `<figure class="trn-flow">`) {
			t.Errorf("%s: RenderDocumentView(ViewTable) did not isolate table view", name)
		}
		docFlow := string(RenderDocumentView(r, ViewFlow))
		if !strings.Contains(docFlow, `<figure class="trn-flow">`) || strings.Contains(docFlow, `<table class="trn-table">`) {
			t.Errorf("%s: RenderDocumentView(ViewFlow) did not isolate flow view", name)
		}
		if got := RenderView(r, ViewTable); got != RenderTable(r) {
			t.Errorf("%s: RenderView(ViewTable) != RenderTable", name)
		}
		if got := RenderView(r, ViewFlow); got != RenderFlow(r) {
			t.Errorf("%s: RenderView(ViewFlow) != RenderFlow", name)
		}
	}
}

// TestDocumentTitleFallback covers the empty-title case (W6): a generic title
// instead of an empty heading or an empty <title> element.
func TestDocumentTitleFallback(t *testing.T) {
	doc := string(RenderDocument(&Recipe{Steps: []Step{{Number: 1, Verb: "Mash", Items: []Item{{Text: "1 avocado"}}}}}))
	if !strings.Contains(doc, "<title>"+defaultDocTitle+"</title>") {
		t.Errorf("empty recipe title did not fall back to %q", defaultDocTitle)
	}
	if !strings.Contains(doc, `<h1 class="trn-doc-title">`+defaultDocTitle+`</h1>`) {
		t.Errorf("empty recipe title did not fall back in the page heading")
	}
}

// TestDocumentEscaping pushes markup, quotes and Unicode through the title and
// every text field: the page must show the bytes and never execute them.
func TestDocumentEscaping(t *testing.T) {
	title := `Chai & "match" </title><script>alert(1)</script> — 'tis`
	r := &Recipe{
		Title: title,
		Steps: []Step{
			{Number: 1, Verb: `<b>Crush</b>`, Items: []Item{{Text: `<img src=x onerror=alert(1)>`}}},
			{Number: 2, Verb: "Boil", Items: []Item{{Ref: 1}, {Text: "1/2 inch ginger & 3g tea"}}},
		},
	}
	doc := string(RenderDocument(r))
	for _, raw := range []string{"<script>", "</title><script", "<b>Crush</b>", "<img src=x"} {
		if strings.Contains(doc, raw) {
			t.Errorf("user text %q survived unescaped", raw)
		}
	}
	if !strings.Contains(doc, "&lt;script&gt;") {
		t.Error("the title's <script> is not HTML-escaped")
	}
	if !strings.Contains(doc, "1/2 inch ginger &amp; 3g tea") {
		t.Error("ingredient bytes did not survive into the document")
	}
	if n := strings.Count(doc, "<script"); n != 0 {
		t.Errorf("document contains %d script tags", n)
	}
}

// TestRenderDocumentNil: a nil recipe still produces a valid, empty page —
// never a panic, never half a document.
func TestRenderDocumentNil(t *testing.T) {
	doc := string(RenderDocument(nil))
	if string(RenderDocument(nil)) != doc {
		t.Error("RenderDocument(nil) is not deterministic")
	}
	for _, want := range []string{"<!doctype html>", `<html lang="` + defaultLang + `">`, "</html>\n"} {
		if !strings.Contains(doc, want) {
			t.Errorf("nil document is missing %q", want)
		}
	}
	if strings.Contains(doc, `<table class="trn-table">`) {
		t.Error("nil document should carry an empty view, not a table")
	}
}

// TestGoldenDocument keeps one committed standalone page under test, so the
// E-A1 artifact a browser opens is the renderer's actual output.
// Rebuild with `go test ./... -update`.
func TestGoldenDocument(t *testing.T) {
	golden := filepath.Join(outputDir, "chai.doc.html")
	got := string(RenderDocument(loadRecipe(t, "chai.trn.md")))
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./... -update` to create it)", golden, err)
	}
	if got != string(want) {
		t.Errorf("document differs from %s%s", golden, firstDiff(string(want), got))
	}
}

// htmlText escapes s the way html/template does in text context, for tests that
// compare against a rendered value.
func htmlText(t *testing.T, s string) string {
	t.Helper()
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return esc.Replace(s)
}
