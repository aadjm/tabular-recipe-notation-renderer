package trn

import (
	"bytes"
	"html/template"
)

// View options for RenderView and RenderDocumentView.
const (
	ViewTable = "table"
	ViewFlow  = "flow"
	ViewBoth  = "both"
)

const (
	defaultLang     = "en"
	defaultDocTitle = "Recipe"
)

// RenderView renders the requested view fragment ("table", "flow", or "both").
// An unrecognized or empty view defaults to ViewBoth.
func RenderView(r *Recipe, view string) template.HTML {
	if r == nil {
		return ""
	}
	switch view {
	case ViewTable:
		return RenderTable(r)
	case ViewFlow:
		return RenderFlow(r)
	default:
		table := RenderTable(r)
		flow := RenderFlow(r)
		if table == "" {
			return flow
		}
		if flow == "" {
			return table
		}
		return table + "\n" + flow
	}
}

// RenderDocument renders r as a complete, plain HTML page containing both views.
// No "Table" or "Flow" section headers are emitted.
func RenderDocument(r *Recipe) template.HTML {
	return RenderDocumentView(r, ViewBoth)
}

// RenderDocumentView renders r as a complete, plain HTML page containing only
// the selected view ("table", "flow", or "both"), so callers can toggle and
// render each view separately or together.
func RenderDocumentView(r *Recipe, view string) template.HTML {
	data := documentView{
		Lang:  defaultLang,
		Title: defaultDocTitle,
	}
	if r != nil && r.Title != "" {
		data.Title = r.Title
	}
	switch view {
	case ViewTable:
		data.Table = RenderTable(r)
	case ViewFlow:
		data.Flow = RenderFlow(r)
	default:
		data.Table = RenderTable(r)
		data.Flow = RenderFlow(r)
	}
	var buf bytes.Buffer
	if err := documentTemplate.Execute(&buf, data); err != nil {
		return ""
	}
	return template.HTML(buf.String())
}

// documentView is the data set handed to the document template.
type documentView struct {
	Lang  string        // <html lang="…">
	Title string        // <title> and the page <h1>
	Table template.HTML // the matrix view fragment (empty when Flow-only)
	Flow  template.HTML // the flow view fragment (empty when Table-only)
}

// documentTemplate is the standalone page: literal markup only, no stylesheet,
// no controls, and no "Table" or "Flow" section headers.
var documentTemplate = template.Must(template.New("trn.document").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
</head>
<body class="trn-doc">
<h1 class="trn-doc-title">{{.Title}}</h1>
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
