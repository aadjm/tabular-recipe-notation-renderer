package trn

import (
	"bytes"
	"html/template"
)

// This file is the shared presentation layer of SPEC-ENGINE §4.3: the palette,
// the system font stack and the print baseline the standalone document builds
// on, the CSS-only view switcher, and the document the CLI's --standalone
// wraps around the two fragments.
//
// The constraints are structural, not stylistic, and everything here keeps
// them:
//
//   - Zero JavaScript. The switcher is a pair of radio inputs styled by
//     :checked; there is no script element, no event-handler attribute and no
//     <noscript> fallback to depend on.
//   - No external resources. Every byte of CSS in this file is embedded in the
//     output, the family stack is system fonts only, and there is no @font-face,
//     <link>, src= or url() anywhere.
//   - Light and dark come from CSS custom properties, flipped by
//     prefers-color-scheme; print flips them to black on white.
//   - Print is part of the same stylesheet: screen chrome (the switcher) is
//     hidden, the tabular view is forced visible and the table header repeats on
//     every printed page.
//
// The two view fragments are pure HTML and carry none of this: the baseline
// lives only in the standalone page, composed with the page chrome, so a
// fragment embedded by the site cannot smuggle styling into it. The source of
// the baseline is this file and sharedCSS is the only copy.

// defaultLang is the lang attribute of the standalone document. The dialect
// carries no language information of its own, the site owns its own <html>
// element, and the CLI (SPEC-ENGINE §5) defines no flag for it — so the engine
// states the language it serves, English, rather than omitting the attribute
// (which would leave every screen reader guessing at pronunciation).
const defaultLang = "en"

// defaultDocTitle is the <title> and <h1> of a document whose recipe has no
// title (W6). A generic title beats an empty one in a tab, a bookmark and a
// heading outline.
const defaultDocTitle = "Recipe"

// sharedCSS is the presentation baseline: palette, system font stack, dark mode
// and the print reset. The standalone page embeds it verbatim (composed with
// documentCSS), and TestSharedStylesheetIsSingleSource checks both that the page
// carries exactly this block and that neither view fragment carries any, so a
// re-skin changes this one constant.
//
// Every foreground/background pair the views use is checked by
// TestPaletteContrast, which reads the hex values back out of this block:
// text pairs clear 4.5:1, the CSS-generated glyphs clear 3:1.
//
// The markers are inert CSS comments; they exist so the shared block can be
// located in a rendered fragment (by the tests, and by anyone auditing a
// deployment's stylesheet).
const sharedCSS = `/* trn:shared:start */
.trn{--trn-fg:#1c1b19;--trn-bg:#fffefb;--trn-line:#cdc7bb;--trn-head:#f2eee4;--trn-direct:#fff8e4;--trn-mark:#9a6b1f;--trn-muted:#6b655b;box-sizing:border-box;max-width:100%;color:var(--trn-fg);background:var(--trn-bg);font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;font-size:.95rem;line-height:1.35}
@media (prefers-color-scheme:dark){.trn{--trn-fg:#e9e5dc;--trn-bg:#151412;--trn-line:#3b3833;--trn-head:#24211c;--trn-direct:#2a2517;--trn-mark:#e0b463;--trn-muted:#a49c8e}}
@media print{.trn{--trn-fg:#000000;--trn-bg:#ffffff;--trn-line:#666666;--trn-head:#eeeeee;--trn-direct:#ffffff;--trn-mark:#000000;--trn-muted:#333333;color:#000;background:#fff;font-size:10pt}}
/* trn:shared:end */
`

// RenderDocument renders r as one complete, standalone HTML page: the visible
// title, a CSS-only switcher between the two views, and both fragments in the
// same document (SPEC-ENGINE §4.3, §5 `--standalone`). This is what the
// trn-render CLI wraps a recipe in, and nothing else in the engine needs it —
// the site embeds RenderTable / RenderFlow directly and owns its own document.
//
// The page is self-contained: no script, no external stylesheet, no webfont, no
// image — a single file:// URL renders both views and makes no network request.
// It states its language (`lang`), keeps a logical heading order (h1 title →
// h2 view sections → h3 step boxes) and gives the tabular view a print
// stylesheet, so a printed recipe is the matrix, not the screen chrome.
//
// RenderDocument never panics and never errors: a nil recipe renders the page
// shell with empty views, and a recipe without a title falls back to
// defaultDocTitle instead of an empty heading. To override the title, set
// Recipe.Title before calling.
func RenderDocument(r *Recipe) template.HTML {
	data := documentView{
		Lang:  defaultLang,
		Title: defaultDocTitle,
		// The page is the only thing in the engine that still carries a
		// stylesheet, and the chrome rules below need the shared palette
		// custom properties to mean anything — so the baseline is composed in
		// here instead of riding along inside a view fragment (both views are
		// pure HTML now, SPEC-ENGINE §4.3).
		CSS:   template.CSS(sharedCSS + documentCSS),
		Table: RenderTable(r),
		Flow:  RenderFlow(r),
	}
	if r != nil && r.Title != "" {
		data.Title = r.Title
	}
	var buf bytes.Buffer
	if err := documentTemplate.Execute(&buf, data); err != nil {
		// Unreachable: compile-time constant template, plain strings and
		// fragments that are already HTML. Degrade to an empty page rather than
		// panic, like the view renderers do.
		return ""
	}
	// Safe by construction: the title went through html/template's escaper, the
	// two fragments are this package's own template output, and the rest is
	// this file's markup and CSS.
	return template.HTML(buf.String())
}

// documentView is the data set handed to the document template.
type documentView struct {
	Lang  string        // <html lang="…">
	Title string        // <title> and the page <h1>
	CSS   template.CSS  // the document chrome's own stylesheet
	Table template.HTML // the matrix view fragment
	Flow  template.HTML // the flow view fragment
}

// documentCSS styles the page chrome only — title, switcher, panels, and what
// print does with them. It is composed after sharedCSS in the same <style>
// element, so the palette custom properties it reads are declared once, by the
// baseline, and every colour in the page comes from there. The view fragments
// carry no styling at all: this file is the only place the engine still emits
// CSS, and it exists for `--standalone` output only.
//
// The switcher is a radio group, not a checkbox or :target pair: two views are
// mutually exclusive, and a radio group already means exactly that — one
// checked, arrow keys move between them, the label toggles the control, and
// assistive technology announces a selected state. :checked is the only
// selector involved; no script, no hidden input state.
//
// Layout rule the markup depends on: the inputs, their labels and the two
// panels must stay siblings inside .trn-views, because the panel rules are
// `input:checked ~ panel` sibling selectors.
//
// The radios are visually hidden but NOT display:none, so they stay focusable;
// the focus ring is drawn on the label instead. In print the switcher is
// removed and the tabular view is forced visible whatever is selected: a
// printed recipe is the matrix.
const documentCSS = `:root{color-scheme:light dark}
.trn-doc{margin:0;padding:1.25rem}
.trn-doc-title{margin:0 0 .9rem;font-size:1.35rem;line-height:1.25}
.trn-views{position:relative}
.trn-sr-only{position:absolute;width:1px;height:1px;margin:-1px;padding:0;overflow:hidden;clip-path:inset(50%);white-space:nowrap;border:0}
.trn-view-toggle{position:absolute;width:1px;height:1px;margin:-1px;padding:0;overflow:hidden;clip-path:inset(50%);white-space:nowrap;border:0}
.trn-tab{display:inline-block;margin:0 .3rem 0 0;padding:.35rem .8rem;border:1px solid var(--trn-line);border-bottom:0;border-radius:.4rem .4rem 0 0;background:var(--trn-head);color:var(--trn-muted);cursor:pointer;font-size:.9rem;font-weight:500}
.trn-view-toggle:checked+.trn-tab{background:var(--trn-bg);color:var(--trn-fg);font-weight:600;border-color:var(--trn-mark)}
.trn-view-toggle:focus-visible+.trn-tab{outline:2px solid var(--trn-mark);outline-offset:2px}
.trn-panel{border:1px solid var(--trn-line);border-radius:0 .4rem .4rem;padding:.6rem}
.trn-panel-flow{display:none}
#trn-tab-flow:checked~.trn-panel-table{display:none}
#trn-tab-flow:checked~.trn-panel-flow{display:block}
.trn-doc .trn-title{position:absolute;width:1px;height:1px;margin:-1px;padding:0;overflow:hidden;clip-path:inset(50%);white-space:nowrap;border:0}
@media print{.trn-doc{padding:0}.trn-view-toggle,.trn-tab{display:none}.trn-panel{border:0;border-radius:0;padding:0}.trn-panel-flow{display:none!important}.trn-panel-table{display:block!important}}`

// documentTemplate is the standalone page. Literal markup lives here; the title
// is an action, so html/template escapes it for both the RCDATA <title> and the
// body heading, and the two view fragments are inserted as HTML because they
// are already this package's escaped output.
//
// Heading order: h1 (title) → h2 (one per view panel, visually hidden because
// the tabs are its visible name) → h3 (the flow view's step boxes). The panels
// are labelled by their tab via aria-labelledby, and the views' own
// caption/figcaption titles are visually hidden in this context so the title is
// not read or shown twice while still naming the table for assistive
// technology.
var documentTemplate = template.Must(template.New("trn.document").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
{{.CSS}}
</style>
</head>
<body class="trn trn-doc">
<h1 class="trn-doc-title">{{.Title}}</h1>
<div class="trn-views">
<input type="radio" name="trn-view" id="trn-tab-table" class="trn-view-toggle" checked>
<label for="trn-tab-table" id="trn-label-table" class="trn-tab">Table</label>
<input type="radio" name="trn-view" id="trn-tab-flow" class="trn-view-toggle">
<label for="trn-tab-flow" id="trn-label-flow" class="trn-tab">Flow</label>
<section class="trn-panel trn-panel-table" id="trn-panel-table" aria-labelledby="trn-label-table">
<h2 class="trn-panel-heading trn-sr-only">Table</h2>
{{.Table}}
</section>
<section class="trn-panel trn-panel-flow" id="trn-panel-flow" aria-labelledby="trn-label-flow">
<h2 class="trn-panel-heading trn-sr-only">Flow</h2>
{{.Flow}}
</section>
</div>
</body>
</html>
`))
