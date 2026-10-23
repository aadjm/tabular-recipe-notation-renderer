package trn

import (
	"bytes"
	"html/template"
)

// This file is the document wrapper of SPEC-ENGINE §4.3/§5: RenderDocument puts
// the two view fragments into one complete HTML page for the CLI's
// `--standalone` mode.
//
// The page is structure, not appearance — the same rule the fragments follow.
// It carries no <style>, no <script>, no <input>, no CSS custom property, no
// external resource: a classed document, a title and the two views, so the
// embedding site (or the reader's browser defaults) decides how it looks. The
// view switcher, dark mode and print rules are site features (SPEC-SITE §7),
// not engine features; the standalone page simply shows both views in order.

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

// RenderDocument renders r as one complete, plain HTML page: the recipe title
// as the page heading, then the matrix and the flow view in document order,
// each under its own h2 section (SPEC-ENGINE §4.3, §5 `--standalone`). This is
// what the trn-render CLI writes for `--standalone`; the site does not need it,
// because the site owns its own page and embeds RenderTable / RenderFlow.
//
// The page declares its language (`lang`), keeps a logical heading order (h1
// title → h2 view → h3 flow step boxes) and is script-free and self-contained:
// a single file:// URL renders it and makes no network request. It embeds no
// stylesheet — styling, the view toggle, dark mode and the print stylesheet all
// belong to the embedding site (SPEC-SITE §7), so the engine's class names are
// the only thing a stylesheet needs.
//
// Both fragments keep their own caption/figcaption, so an unstyled standalone
// page shows the recipe title more than once: the h1 names the page, the
// caption names the table, the figcaption names the figure. That is a labelling
// duty, not duplication to be styled away; a site rendering the fragments in
// its own page hides its own copies as it sees fit.
//
// RenderDocument never panics and never errors: a nil recipe renders the page
// shell with empty views, and a recipe without a title falls back to
// defaultDocTitle instead of an empty heading. To override the title, set
// Recipe.Title before calling.
func RenderDocument(r *Recipe) template.HTML {
	data := documentView{
		Lang:  defaultLang,
		Title: defaultDocTitle,
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
	// this file's markup.
	return template.HTML(buf.String())
}

// documentView is the data set handed to the document template.
type documentView struct {
	Lang  string        // <html lang="…">
	Title string        // <title> and the page <h1>
	Table template.HTML // the matrix view fragment
	Flow  template.HTML // the flow view fragment
}

// documentTemplate is the standalone page: literal markup only, no stylesheet
// and no controls. The title is an action, so html/template escapes it for both
// the RCDATA <title> and the body heading, and the two view fragments are
// inserted as HTML because they are already this package's escaped output.
//
// Heading order: h1 (title) → h2 (one per view section) → h3 (the flow view's
// step boxes). Each section is named by its own h2, which is what makes it a
// labelled region for assistive technology; nothing here needs a switcher,
// because both views are simply present in order.
var documentTemplate = template.Must(template.New("trn.document").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
</head>
<body class="trn-doc">
<h1 class="trn-doc-title">{{.Title}}</h1>
<section class="trn-doc-view trn-doc-view-table">
<h2 class="trn-doc-heading">Table</h2>
{{.Table}}
</section>
<section class="trn-doc-view trn-doc-view-flow">
<h2 class="trn-doc-heading">Flow</h2>
{{.Flow}}
</section>
</body>
</html>
`))
