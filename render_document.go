package trn

import (
	"bytes"
	"html/template"
)

// View options for RenderView, RenderDocumentView, and RenderOptions.
const (
	ViewTable = "table"
	ViewFlow  = "flow"
	ViewBoth  = "both"
)

const (
	defaultLang     = "en"
	defaultDocTitle = "Recipe"
)

// RenderView renders the requested view fragment ("table", "flow", or "both")
// with the title hidden by default.
func RenderView(r *Recipe, view string) template.HTML {
	return RenderWithOptions(r, RenderOptions{View: view, ShowTitle: false})
}

// RenderWithOptions renders the requested view fragment ("table", "flow", or
// "both") with configurable title visibility.
func RenderWithOptions(r *Recipe, opts RenderOptions) template.HTML {
	if r == nil {
		return ""
	}
	switch opts.View {
	case ViewTable:
		return RenderTableWithTitle(r, opts.ShowTitle)
	case ViewFlow:
		return RenderFlowWithTitle(r, opts.ShowTitle)
	default:
		table := RenderTableWithTitle(r, opts.ShowTitle)
		flow := RenderFlowWithTitle(r, opts.ShowTitle)
		if table == "" {
			return flow
		}
		if flow == "" {
			return table
		}
		return table + "\n" + flow
	}
}

// RenderDocument renders r as a complete, plain HTML page containing both views
// with the visible title hidden by default.
func RenderDocument(r *Recipe) template.HTML {
	return RenderDocumentWithOptions(r, RenderOptions{View: ViewBoth, ShowTitle: false})
}

// RenderDocumentView renders r as a complete, plain HTML page containing only
// the selected view ("table", "flow", or "both") with the visible title hidden
// by default.
func RenderDocumentView(r *Recipe, view string) template.HTML {
	return RenderDocumentWithOptions(r, RenderOptions{View: view, ShowTitle: false})
}

// RenderDocumentWithOptions renders r as a complete, plain HTML page with
// configurable view selection ("table", "flow", or "both") and title visibility
// (ShowTitle). When ShowTitle is false (the default), the visible <h1> heading
// and fragment captions are omitted.
func RenderDocumentWithOptions(r *Recipe, opts RenderOptions) template.HTML {
	data := documentView{
		Lang:      defaultLang,
		Title:     defaultDocTitle,
		ShowTitle: opts.ShowTitle,
	}
	if r != nil && r.Title != "" {
		data.Title = r.Title
	}
	// In standalone document mode, if ShowTitle is true we show the page-level
	// <h1 class="trn-doc-title"> and avoid duplicating it inside inner captions,
	// or show inner captions as well; keeping inner captions false when wrapped
	// in a document avoids duplicate titles when ShowTitle is enabled, while
	// fragment calls (RenderWithOptions) emit caption/figcaption when ShowTitle is true.
	switch opts.View {
	case ViewTable:
		data.Table = RenderTableWithTitle(r, false)
	case ViewFlow:
		data.Flow = RenderFlowWithTitle(r, false)
	default:
		data.Table = RenderTableWithTitle(r, false)
		data.Flow = RenderFlowWithTitle(r, false)
	}
	var buf bytes.Buffer
	if err := documentTemplate.Execute(&buf, data); err != nil {
		return ""
	}
	return template.HTML(buf.String())
}

// documentView is the data set handed to the document template.
type documentView struct {
	Lang      string        // <html lang="…">
	Title     string        // <title> and (when ShowTitle is true) the page <h1>
	ShowTitle bool          // whether to emit the visible <h1 class="trn-doc-title">
	Table     template.HTML // the matrix view fragment (empty when Flow-only)
	Flow      template.HTML // the flow view fragment (empty when Table-only)
}

// documentTemplate is the standalone page: literal markup only, no stylesheet,
// no controls, and no "Table" or "Flow" section headers. The visible <h1> title
// is emitted only when ShowTitle is true (default false).
var documentTemplate = template.Must(template.New("trn.document").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
</head>
<body class="trn-doc">
{{- if .ShowTitle}}
<h1 class="trn-doc-title">{{.Title}}</h1>
{{- end}}
{{- if .Table}}
<section class="trn-doc-view trn-doc-view-table">
{{.Table}}</section>
{{- end}}
{{- if .Flow}}
<section class="trn-doc-view trn-doc-view-flow">
{{.Flow}}</section>
{{- end}}
</body>
</html>
`))
