package trn

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// markedShared returns the text between the shared-CSS markers, i.e. the block
// every rendered fragment and the standalone document must carry verbatim.
func markedShared(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, "/* trn:shared:start */")
	end := strings.Index(html, "/* trn:shared:end */")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("no shared stylesheet block in output (start=%d end=%d)", start, end)
	}
	return html[start : end+len("/* trn:shared:end */")]
}

// TestSharedStylesheetIsSingleSource is the structural half of §4.3's "all CSS
// embedded, one presentation rule set": the two views and the standalone
// document each embed the same baseline, byte for byte, and that baseline is
// the sharedCSS constant rather than a copy that can drift.
func TestSharedStylesheetIsSingleSource(t *testing.T) {
	want := markedShared(t, sharedCSS)
	if rest := strings.TrimPrefix(sharedCSS, want); rest != "\n" {
		t.Fatalf("sharedCSS is more than the marked block plus a trailing newline: %q", rest)
	}

	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		for view, fragment := range map[string]string{
			"table": string(RenderTable(r)),
			"flow":  string(RenderFlow(r)),
			"doc":   string(RenderDocument(r)),
		} {
			if got := markedShared(t, fragment); got != want {
				t.Errorf("%s/%s: embedded presentation baseline differs from sharedCSS", name, view)
			}
		}
	}

	// The baseline must be the only place the palette is declared: a view that
	// re-declares --trn-fg would silently win over the shared block.
	if n := strings.Count(string(RenderTable(&Recipe{})), "--trn-fg:"); n != 3 {
		t.Errorf("light/dark/print --trn-fg declarations = %d, want 3 (one per mode)", n)
	}
	if n := strings.Count(string(RenderFlow(&Recipe{})), "--trn-fg:"); n != 3 {
		t.Errorf("flow --trn-fg declarations = %d, want 3 (one per mode)", n)
	}
}

// TestDocumentNoScriptNoExternal is the static half of acceptance criterion
// E-A1: the standalone page contains no script, no event-handler attribute, and
// no reference that could leave the machine — no stylesheet or script link, no
// src, no webfont, no image, no url(), no absolute URL.
func TestDocumentNoScriptNoExternal(t *testing.T) {
	for _, name := range fixtureNames(t) {
		doc := string(RenderDocument(loadRecipe(t, name)))
		for _, bad := range []string{
			"<script", "</script", "<noscript", "<img", "<svg", "<canvas",
			"<iframe", "<object", "<embed", "<link", "<video", "<audio",
			"src=", "javascript:", "<a ", "href=", "@import", "url(",
			"@font-face", "http://", "https://", "//cdn",
			"onload=", "onclick=", "onerror=", "onchange=", "onsubmit=",
			"onmouseover=", "onfocus=",
		} {
			if strings.Contains(doc, bad) {
				t.Errorf("%s: document contains %q; the page must be script-free and self-contained", name, bad)
			}
		}
		if !strings.Contains(doc, "<style>") {
			t.Errorf("%s: document does not embed its stylesheet", name)
		}
	}
}

// TestDocumentSwitcherIsCSSOnly proves the view switcher needs no script: it is
// a radio group whose :checked state drives the panels through sibling
// selectors, with every label wired to its input by for=/id=.
func TestDocumentSwitcherIsCSSOnly(t *testing.T) {
	doc := string(RenderDocument(loadRecipe(t, "grocy-1.trn.md")))

	if n := strings.Count(doc, `name="trn-view"`); n != 2 {
		t.Fatalf("switcher radio count = %d, want 2", n)
	}
	if n := strings.Count(doc, " checked"); n != 1 {
		t.Errorf("default-checked inputs = %d, want exactly 1 (the tabular view)", n)
	}
	if !strings.Contains(doc, `id="trn-tab-table" class="trn-view-toggle" checked`) {
		t.Error("the tabular view is not the default selection")
	}

	for _, id := range []string{"trn-tab-table", "trn-tab-flow"} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Errorf("missing input id %q", id)
		}
		if !strings.Contains(doc, `for="`+id+`"`) {
			t.Errorf("missing label for input %q", id)
		}
	}

	// The state machine has to be pure CSS and has to be in the page.
	for _, rule := range []string{
		".trn-panel-flow{display:none}",
		"#trn-tab-flow:checked~.trn-panel-table{display:none}",
		"#trn-tab-flow:checked~.trn-panel-flow{display:block}",
		".trn-view-toggle:checked+.trn-tab",
		".trn-view-toggle:focus-visible+.trn-tab",
	} {
		if !strings.Contains(doc, rule) {
			t.Errorf("switcher stylesheet is missing rule %q", rule)
		}
	}

	// The sibling selectors only work while the inputs precede the panels in
	// the same parent, so the markup order is part of the contract.
	lastInput := strings.LastIndex(doc, `type="radio"`)
	firstPanel := strings.Index(doc, `class="trn-panel trn-panel-table"`)
	if lastInput < 0 || firstPanel < 0 || lastInput > firstPanel {
		t.Errorf("inputs and panels are not siblings in the required order (last input %d, first panel %d)", lastInput, firstPanel)
	}
}

// TestDocumentStructure checks the accessibility contract of §4.3 on the
// generated page: a language, one h1 with the recipe title, a logical heading
// order, labelled panels, a real table with scoped headers, and both views
// present in one document.
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
		if n := strings.Count(doc, "<h2"); n != 2 {
			t.Errorf("%s: h2 count = %d, want 2 (one per view panel)", name, n)
		}
		if !strings.Contains(doc, `<h1 class="trn-doc-title">`+htmlText(t, r.Title)+`</h1>`) {
			t.Errorf("%s: page heading does not carry the recipe title", name)
		}

		// Heading order: never skip a level going down.
		levels := headingLevels(doc)
		if len(levels) == 0 || levels[0] != 1 {
			t.Fatalf("%s: heading levels start with %v, want h1 first", name, levels)
		}
		for i := 1; i < len(levels); i++ {
			if levels[i] > levels[i-1]+1 {
				t.Errorf("%s: heading order skips a level: %v", name, levels)
				break
			}
		}

		// Panels are named by their tab, and the tabs are the switcher labels.
		for _, pair := range [][2]string{
			{`id="trn-panel-table"`, `aria-labelledby="trn-label-table"`},
			{`id="trn-panel-flow"`, `aria-labelledby="trn-label-flow"`},
		} {
			if !strings.Contains(doc, pair[0]) || !strings.Contains(doc, pair[1]) {
				t.Errorf("%s: panel/label pairing %v is missing", name, pair)
			}
		}
		if !strings.Contains(doc, `<label for="trn-tab-table" id="trn-label-table"`) {
			t.Errorf("%s: table tab label is not linked and named", name)
		}

		// A real table, scoped headers, and both views in the same page.
		for _, want := range []string{
			`<table class="trn-table">`,
			`<th scope="col"`,
			`<th scope="row"`,
			`<figure class="trn trn-view trn-view-flow">`,
		} {
			if !strings.Contains(doc, want) {
				t.Errorf("%s: document is missing %q", name, want)
			}
		}
		if strings.Index(doc, "trn-view-table") > strings.Index(doc, "trn-view-flow") {
			t.Errorf("%s: the tabular view must come first (it is the default)", name)
		}

		// The title is shown once by the page; the fragments keep their own
		// caption/figcaption for standalone use, hidden in this context so the
		// title is not duplicated on screen.
		if !strings.Contains(doc, `.trn-doc .trn-title{position:absolute`) {
			t.Errorf("%s: the fragments' own titles are not hidden in document context", name)
		}
	}
}

// TestDocumentPrint checks criterion 2: a print stylesheet is present and it
// makes the tabular view the printed view, with no screen-only chrome.
func TestDocumentPrint(t *testing.T) {
	doc := string(RenderDocument(loadRecipe(t, "grocy-1.trn.md")))
	// The print rules live in three stylesheets — the page chrome, the shared
	// baseline and each view — so the check is over the whole document.
	style := doc

	for _, rule := range []string{
		// shared baseline: black on white, and a print type size
		"@media print{.trn{--trn-fg:#000",
		"font-size:10pt",
		// chrome removed, tabular view forced, flow view suppressed
		"@media print{.trn-doc{padding:0}",
		".trn-view-toggle,.trn-tab{display:none}",
		".trn-panel-flow{display:none!important}",
		".trn-panel-table{display:block!important}",
		// the matrix prints as a table on a landscape page: it fits, the header
		// repeats, and rows are not split across pages
		"@media print{@page{size:landscape;margin:10mm}",
		".trn-view-table .trn-table{font-size:8pt;table-layout:fixed;width:100%}",
		".trn-view-table .trn-table th:first-child{width:15%}",
		".trn-table thead th{white-space:normal}",
		".trn-table thead{display:table-header-group}",
		".trn-table tr{break-inside:avoid",
		".trn-view-table{overflow:visible;padding:0}",
	} {
		if !strings.Contains(style, rule) {
			t.Errorf("print stylesheet is missing %q", rule)
		}
	}

	// The print rules must live in the view fragment, not only in the page
	// chrome: the site embeds the fragment and prints its own page.
	fragment := string(RenderTable(loadRecipe(t, "grocy-1.trn.md")))
	for _, rule := range []string{
		"@media print{@page{size:landscape;margin:10mm}",
		".trn-table thead{display:table-header-group}",
	} {
		if !strings.Contains(fragment, rule) {
			t.Errorf("table fragment is missing its print rule %q", rule)
		}
	}

	// The forced-visibility rule has to beat the :checked rules, which are
	// ID-level selectors: only !important can, so both halves must be present.
	if strings.Contains(doc, "#trn-tab-flow:checked~.trn-panel-table{display:none!important}") {
		t.Error("the print rule cannot be overridden by the switcher: !important is on the wrong side")
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
	title := `Chai & "match" </title><script>alert(1)</script> — ’tis`
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
	if strings.Contains(doc, "<table class=\"trn-table\">") {
		t.Error("nil document should carry an empty view, not a table")
	}
}

// TestGoldenDocument keeps one committed standalone page under test, so the
// E-A1 artifact the browser check opens is the renderer's actual output.
// Rebuild with `go test ./... -update`.
func TestGoldenDocument(t *testing.T) {
	golden := filepath.Join("testdata", "chai.doc.html")
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

// TestPaletteContrast audits §4.3's "sufficient contrast" against the hex
// values actually emitted, so a palette tweak cannot quietly fail WCAG:
// text pairs must clear 4.5:1 (1.4.3) and the CSS-generated glyphs must clear
// 3:1 (1.4.11).
func TestPaletteContrast(t *testing.T) {
	frag := string(RenderTable(&Recipe{}))
	lines := strings.Split(markedShared(t, frag), "\n")
	blocks := map[string]string{}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, ".trn{"):
			blocks["light"] = line
		case strings.Contains(line, "prefers-color-scheme:dark"):
			blocks["dark"] = line
		case strings.Contains(line, "@media print{.trn{"):
			blocks["print"] = line
		}
	}
	palettes := make(map[string]map[string]string, len(blocks))
	for mode, block := range blocks {
		palettes[mode] = parsePalette(t, block)
	}
	if len(palettes) != 3 {
		t.Fatalf("found %d palette blocks (%v), want light, dark and print", len(palettes), blocks)
	}

	// Text is read on the page background, on the box/header background and on
	// the "listed here" cell background; the markers are glyphs, not text.
	textPairs := [][2]string{{"trn-fg", "trn-bg"}, {"trn-fg", "trn-head"}, {"trn-fg", "trn-direct"}, {"trn-muted", "trn-bg"}, {"trn-muted", "trn-head"}, {"trn-muted", "trn-direct"}}
	for _, mode := range []string{"light", "dark", "print"} {
		p := palettes[mode]
		for _, pair := range textPairs {
			got := contrast(t, p, pair[0], pair[1])
			if got < 4.5 {
				t.Errorf("%s: %s on %s contrast = %.2f:1, want >= 4.5:1", mode, pair[0], pair[1], got)
			}
		}
		for _, bg := range []string{"trn-bg", "trn-head", "trn-direct"} {
			got := contrast(t, p, "trn-mark", bg)
			if got < 3 {
				t.Errorf("%s: trn-mark on %s contrast = %.2f:1, want >= 3:1 (non-text glyph)", mode, bg, got)
			}
		}
	}
}

// parsePalette reads the `--trn-*:#rrggbb` custom properties out of one CSS
// block.
func parsePalette(t *testing.T, block string) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`--(trn-[a-z]+):(#[0-9a-fA-F]{6})`)
	p := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		p[m[1]] = m[2]
	}
	if len(p) == 0 {
		t.Fatalf("no custom properties in palette block %q", block)
	}
	return p
}

// contrast is the WCAG 2.x contrast ratio between two palette entries.
func contrast(t *testing.T, p map[string]string, a, b string) float64 {
	t.Helper()
	la, ok := p[a]
	if !ok {
		t.Fatalf("palette has no %s", a)
	}
	lb, ok := p[b]
	if !ok {
		t.Fatalf("palette has no %s", b)
	}
	hi, lo := math.Max(luminance(t, la), luminance(t, lb)), math.Min(luminance(t, la), luminance(t, lb))
	return (hi + 0.05) / (lo + 0.05)
}

// luminance is the WCAG relative luminance of a #rrggbb colour.
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		t.Fatalf("colour %q is not #rrggbb", hex)
	}
	var rgb [3]float64
	for i := range rgb {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			t.Fatalf("colour %q: %v", hex, err)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			rgb[i] = c / 12.92
			continue
		}
		rgb[i] = math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}

// headingLevels lists the h1…h6 levels in document order.
func headingLevels(html string) []int {
	re := regexp.MustCompile(`<h([1-6])[ >]`)
	var levels []int
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		levels = append(levels, n)
	}
	return levels
}

// htmlText escapes s the way html/template does in text context, for tests that
// compare against a rendered value.
func htmlText(t *testing.T, s string) string {
	t.Helper()
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;", "—", "—")
	return esc.Replace(s)
}

// sanity: the shared block is non-trivial, otherwise the tests above would pass
// on an empty baseline.
func TestSharedStylesheetIsSubstantial(t *testing.T) {
	for _, want := range []string{
		"--trn-fg:", "--trn-bg:", "--trn-line:", "--trn-head:", "--trn-mark:", "--trn-muted:", "--trn-direct:",
		"system-ui,-apple-system", "prefers-color-scheme:dark", "@media print", "font-size:10pt",
	} {
		if !strings.Contains(sharedCSS, want) {
			t.Errorf("shared baseline is missing %q", want)
		}
	}
	if strings.Contains(sharedCSS, "url(") || strings.Contains(sharedCSS, "@import") || strings.Contains(sharedCSS, "@font-face") {
		t.Error("shared baseline pulls in an external resource")
	}
	if n := len(sharedCSS); n < 400 || n > 2000 {
		t.Errorf("shared baseline is %d bytes; expected a small baseline, not a stylesheet rewrite", n)
	}
}
