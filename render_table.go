package trn

import (
	"bytes"
	"html/template"
)

// RenderTable renders a recipe as the Cooking-for-Engineers matrix view
// (SPEC-ENGINE §4.1): one row per unique ingredient in first-appearance order,
// one column per step in document order, and a spanning action row for every
// step that adds no ingredient of its own.
//
// The returned fragment is self-contained — it carries its own classed markup
// and its own stylesheet (light/dark via prefers-color-scheme, plus a print
// stylesheet), uses no JavaScript, no external resources and no images, so the
// embedding site can either use it as-is or re-skin it via the `trn-*` classes.
//
// All user text (title, verbs, ingredients) goes through html/template's
// escaping paths; ingredient strings keep their bytes and differ only by HTML
// escaping (SPEC-ENGINE §10). RenderTable never panics and never errors: a nil
// recipe renders nothing, a document without steps renders an empty table.
func RenderTable(r *Recipe) template.HTML {
	if r == nil {
		return ""
	}
	var buf bytes.Buffer
	// The shared presentation baseline (presentation.go) travels inside this
	// fragment, so the fragment renders the same alone as it does inside
	// RenderDocument's page.
	err := tableTemplate.Execute(&buf, tableFragment{
		CSS:        template.CSS(sharedCSS),
		matrixView: buildMatrix(r),
	})
	if err != nil {
		// Unreachable: the template is a compile-time constant and the data is
		// plain strings and ints. Degrade to an empty fragment anyway — this
		// package never turns a rendering problem into a panic.
		return ""
	}
	// Safe by construction: every byte of user text in buf was written by
	// html/template's escaper, the rest is this package's own markup and CSS.
	return template.HTML(buf.String())
}

// matrixKind classifies a matrix cell.
//
// The values are the class suffixes the template switches on, so they are part
// of the emitted markup as well as of the model.
const (
	cellEmpty  = "empty"  // ingredient neither listed in the step nor carried into it
	cellDirect = "direct" // ingredient listed in the step: the cell shows the step verb
	cellFlow   = "flow"   // ingredient reaches the step through back-references: flow marker
)

// tableFragment is the data set for the table template: the shared presentation
// baseline plus the matrix model of one recipe.
type tableFragment struct {
	CSS template.CSS
	matrixView
}

// matrixView is the whole data set handed to the table template.
type matrixView struct {
	Title   string
	Steps   []matrixStep   // one column per step, document order
	Rows    []matrixRow    // unique ingredients, first-appearance order
	Actions []matrixAction // steps that add no ingredient, document order
	ColSpan int            // ingredient header column + all step columns
}

type matrixStep struct {
	Number int
	Verb   string
}

type matrixCell struct {
	Kind string // cellEmpty, cellDirect or cellFlow
	Verb string // the step's verb, only for cellDirect
}

type matrixRow struct {
	Text  string
	Cells []matrixCell // one per step, same order as matrixView.Steps
}

// matrixAction is a step whose items are all references (or which has no items
// at all): it introduces nothing new, so it spans the full width of the table.
type matrixAction struct {
	Number int
	Verb   string
	Refs   []stepRef // the steps it names, in written order
}

// stepRef is one reference written in a step, resolved against the document.
// Known is false for a reference to a step that does not exist (already
// reported as W2); such a reference is shown by number only.
//
// Both renderers show references — the matrix on its action rows, the flow view
// on every box — so the type lives here under a view-neutral name rather than
// being duplicated per view.
type stepRef struct {
	Number int
	Verb   string
	Known  bool
}

// buildMatrix derives the matrix model from the AST.
//
// Reachability is precomputed once per column through ancestorSteps, so cells
// are filled in O(steps × items) after that. The whole thing is O(steps² ×
// items): quadratic in the number of steps, which is the right trade for
// documents that have a dozen steps and a dozen ingredients.
func buildMatrix(r *Recipe) matrixView {
	m := matrixView{
		Title:   r.Title,
		ColSpan: 1 + len(r.Steps),
	}
	for _, step := range r.Steps {
		m.Steps = append(m.Steps, matrixStep{Number: step.Number, Verb: step.Verb})
	}

	// reach[col] marks every step that feeds column col, col itself included.
	reach := make([][]bool, len(r.Steps))
	for col := range r.Steps {
		reach[col] = ancestorSteps(r, col)
	}

	// Unique ingredients in document order, plus the steps that list each of
	// them directly. Ingredient identity is the exact item text (the dialect
	// preserves it byte-for-byte), so map keys compare bytes, not case or shape.
	var order []string
	listedIn := make(map[string][]int, len(r.Steps))
	for i, step := range r.Steps {
		for _, item := range step.Items {
			if item.Ref > 0 || item.Text == "" {
				continue // references carry no ingredient text
			}
			if _, seen := listedIn[item.Text]; !seen {
				order = append(order, item.Text)
			}
			listedIn[item.Text] = append(listedIn[item.Text], i)
		}
	}

	m.Rows = make([]matrixRow, 0, len(order))
	for _, text := range order {
		row := matrixRow{Text: text, Cells: make([]matrixCell, len(r.Steps))}
		for col := range r.Steps {
			switch {
			case containsIndex(listedIn[text], col):
				row.Cells[col] = matrixCell{Kind: cellDirect, Verb: r.Steps[col].Verb}
			case anyTrue(reach[col], listedIn[text]):
				row.Cells[col] = matrixCell{Kind: cellFlow}
			default:
				row.Cells[col] = matrixCell{Kind: cellEmpty}
			}
		}
		m.Rows = append(m.Rows, row)
	}

	firstAt := firstByNumber(r)
	for _, step := range r.Steps {
		var refs []int
		addsIngredient := false
		for _, item := range step.Items {
			switch {
			case item.Ref > 0:
				refs = append(refs, item.Ref)
			case item.Text != "":
				addsIngredient = true
			}
		}
		if addsIngredient {
			continue
		}
		action := matrixAction{Number: step.Number, Verb: step.Verb}
		for _, number := range refs {
			ref := stepRef{Number: number}
			if j, ok := firstAt[number]; ok {
				ref.Known = true
				ref.Verb = r.Steps[j].Verb
			}
			action.Refs = append(action.Refs, ref)
		}
		m.Actions = append(m.Actions, action)
	}
	return m
}

// containsIndex reports whether idx occurs in list.
func containsIndex(list []int, idx int) bool {
	for _, v := range list {
		if v == idx {
			return true
		}
	}
	return false
}

// tableTemplate emits the matrix fragment. Literal markup and CSS live here;
// every piece of user text is an action, so html/template escapes it in the
// right context (text, attribute-free, CSS untouched).
//
// The flow marker is a classed, empty span whose arrow glyph comes from CSS:
// the cell carries no text at all, and the glyph is hidden from assistive
// technology rather than read out (the surrounding row and column headers
// already say which ingredient and which step it belongs to).
var tableTemplate = template.Must(template.New("trn.table").Parse(`<div class="trn trn-view trn-view-table">
<style>
{{.CSS}}.trn-view-table{overflow-x:auto;padding:.5rem}
.trn-table{border-collapse:collapse;width:100%}
.trn-table caption{text-align:left;font-weight:600;padding:0 0 .5rem}
.trn-table th,.trn-table td{border:1px solid var(--trn-line);padding:.35rem .5rem;vertical-align:top;text-align:left}
.trn-table thead th{background:var(--trn-head);font-weight:600;white-space:nowrap}
.trn-table tbody th.trn-ingredient{font-weight:500}
.trn-step-num,.trn-action-num{font-variant-numeric:tabular-nums;color:var(--trn-muted)}
.trn-cell-direct{background:var(--trn-direct)}
.trn-mark::before{content:"→";color:var(--trn-mark);font-weight:700;opacity:.85}
.trn-action-cell{background:var(--trn-head);font-weight:500}
.trn-action-verb{font-weight:600}
.trn-action-refs{margin-left:.5rem;color:var(--trn-muted);font-weight:400;font-size:.9em}
.trn-action-refs::before{content:"← ";color:var(--trn-mark)}
.trn-action-ref{display:inline-block}
@media print{@page{size:landscape;margin:10mm}.trn-view-table{overflow:visible;padding:0}.trn-view-table .trn-table{font-size:8pt;table-layout:fixed;width:100%}.trn-view-table .trn-table th:first-child{width:15%}.trn-table th,.trn-table td{padding:.2rem .3rem;overflow-wrap:break-word}.trn-table thead th{white-space:normal}.trn-view-table .trn-table{caption-side:top}.trn-table tr{break-inside:avoid;page-break-inside:avoid}.trn-table thead{display:table-header-group}}
</style>
<table class="trn-table">
{{if .Title}}<caption class="trn-title">{{.Title}}</caption>
{{end}}<thead>
<tr class="trn-head">
<th scope="col" class="trn-corner">Ingredient</th>
{{range .Steps}}<th scope="col" class="trn-step"><span class="trn-step-num">[{{.Number}}]</span> <span class="trn-step-verb">{{.Verb}}</span></th>
{{end}}</tr>
</thead>
<tbody>
{{range .Rows}}<tr class="trn-row">
<th scope="row" class="trn-ingredient">{{.Text}}</th>
{{range .Cells}}{{if eq .Kind "direct"}}<td class="trn-cell trn-cell-direct"><span class="trn-cell-verb">{{.Verb}}</span></td>{{else if eq .Kind "flow"}}<td class="trn-cell trn-cell-flow"><span class="trn-mark" aria-hidden="true"></span></td>{{else}}<td class="trn-cell trn-cell-empty"></td>{{end}}{{end}}
</tr>
{{end}}{{range .Actions}}<tr class="trn-action">
<td class="trn-action-cell" colspan="{{$.ColSpan}}"><span class="trn-action-num">[{{.Number}}]</span> <span class="trn-action-verb">{{.Verb}}</span>{{if .Refs}} <span class="trn-action-refs">{{range .Refs}}<span class="trn-action-ref">{{if .Known}}[{{.Number}}] {{.Verb}}{{else}}#{{.Number}}{{end}}</span> {{end}}</span>{{end}}</td>
</tr>
{{end}}</tbody>
</table>
</div>
`))
