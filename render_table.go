package trn

import (
	"bytes"
	"html/template"
)

// RenderTable renders a recipe as the Cooking-for-Engineers recipe summary
// table (SPEC-ENGINE §4.1): a semantic <table> in which every unique
// ingredient is a row, every step is a column, and each step is ONE cell that
// spans — with rowspan — exactly the ingredient rows it acts on: the
// ingredients it lists plus every ingredient carried into it through a
// back-reference chain.
//
// Steps that add no ingredient of their own (every item is a #reference) are
// not cells in the flow; they render as full-width action rows above the
// ingredient rows — the "preheat the oven" band of the reference format.
//
// The output is structure only: no <style>, no <script>, no view toggle, no
// external resource. Styling and the view switcher belong to the embedding
// site (SPEC-ENGINE §4.3). Every user string goes through html/template's
// escaping paths, so ingredient text keeps its bytes and differs only by HTML
// escaping (SPEC-ENGINE §10).
//
// RenderTable never panics and never errors: a nil recipe renders nothing, a
// document without steps renders an empty table.
func RenderTable(r *Recipe) template.HTML {
	if r == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := tableTemplate.Execute(&buf, buildMatrix(r)); err != nil {
		// Unreachable: the template is a compile-time constant and the data is
		// plain strings and ints. Degrade to an empty fragment rather than
		// panic.
		return ""
	}
	// Safe by construction: every byte of user text in buf was written by
	// html/template's escaper; the rest is this package's own markup.
	return template.HTML(buf.String())
}

// matrixCell.Kind values, emitted as class suffixes as well as used by the
// template.
const (
	cellStep = "step" // a step's cell, spanning the ingredient rows it acts on
	cellGap  = "gap"  // structural spacer keeping a row's cells in their columns
)

// matrixView is the whole data set handed to the table template.
type matrixView struct {
	Title   string
	Steps   []matrixStep   // one column per step, document order (the time axis)
	Rows    []matrixRow    // unique ingredients, first-appearance order
	Actions []matrixAction // steps that add no ingredient, document order
	ColSpan int            // ingredient label column + every step column
}

type matrixStep struct {
	Number int
	Verb   string
}

// matrixCell is one cell of an ingredient row, in grid order. A step cell
// carries the step's verb and the rowspan it was given; a gap cell carries only
// the colspan of the empty grid columns it holds open.
type matrixCell struct {
	Kind string // cellStep or cellGap
	Verb string // the step's verb, only for cellStep
	Span int    // rowspan (cellStep) or colspan (cellGap)
}

type matrixRow struct {
	Text  string
	Cells []matrixCell // in column order, label column excluded
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

// stepSpan is the cell one step contributes to the matrix: the run of
// ingredient rows it spans and the verb it shows. ok is false for a step that
// adds no ingredient of its own; such a step becomes an action row instead.
//
// The span runs from the step's first acted-on ingredient row to its last, so a
// stranded ingredient sitting between two ingredients the step does act on is
// swallowed by the span: an HTML rowspan cannot cover a non-contiguous set, and
// the ingredient is already reported as W4. This is the C4E merging convention —
// what one cell covers is one thing being worked on.
type stepSpan struct {
	ok    bool
	first int
	last  int
	verb  string
}

// buildMatrix derives the matrix model from the AST.
//
// Reachability comes from ancestorSteps, one call per step column (the same
// closure ConvergenceWarnings uses, so the matrix and W4 can never disagree
// about whether an ingredient reaches a step). Everything else is a linear walk
// over the items: the whole build is O(steps² × items), which is the right
// trade for documents that have a dozen steps.
func buildMatrix(r *Recipe) matrixView {
	m := matrixView{
		Title:   r.Title,
		ColSpan: 1 + len(r.Steps),
	}
	for _, step := range r.Steps {
		m.Steps = append(m.Steps, matrixStep{Number: step.Number, Verb: step.Verb})
	}

	// Unique ingredients in document order, plus the steps that list each of
	// them directly. Ingredient identity is the exact item text (the dialect
	// preserves it byte-for-byte), so map keys compare bytes, not case or shape.
	var order []string
	listedIn := make(map[string][]int, len(r.Steps))
	for i, step := range r.Steps {
		for _, item := range step.Items {
			if !isIngredient(item) {
				continue
			}
			if _, seen := listedIn[item.Text]; !seen {
				order = append(order, item.Text)
			}
			listedIn[item.Text] = append(listedIn[item.Text], i)
		}
	}
	rowOf := make(map[string]int, len(order))
	for i, text := range order {
		rowOf[text] = i
	}

	// One cell per ingredient-bearing step: the rows it acts on are the rows of
	// every ingredient listed in a step that feeds it (itself included).
	spans := make([]stepSpan, len(r.Steps))
	for col := range r.Steps {
		if !addsIngredient(r.Steps[col]) {
			continue // action row, not a cell
		}
		reach := ancestorSteps(r, col)
		span := stepSpan{first: len(order), last: -1, verb: r.Steps[col].Verb}
		for i := range r.Steps {
			if i >= len(reach) || !reach[i] {
				continue
			}
			for _, item := range r.Steps[i].Items {
				if !isIngredient(item) {
					continue
				}
				row := rowOf[item.Text]
				if row < span.first {
					span.first = row
				}
				if row > span.last {
					span.last = row
				}
			}
		}
		if span.first <= span.last {
			span.ok = true
			spans[col] = span
		}
	}

	m.Rows = make([]matrixRow, len(order))
	for row, text := range order {
		m.Rows[row] = matrixRow{Text: text, Cells: cellsForRow(spans, row)}
	}

	firstAt := firstByNumber(r)
	for _, step := range r.Steps {
		if addsIngredient(step) {
			continue
		}
		action := matrixAction{Number: step.Number, Verb: step.Verb}
		for _, item := range step.Items {
			if item.Ref <= 0 {
				continue
			}
			ref := stepRef{Number: item.Ref}
			if j, ok := firstAt[item.Ref]; ok {
				ref.Known = true
				ref.Verb = r.Steps[j].Verb
			}
			action.Refs = append(action.Refs, ref)
		}
		m.Actions = append(m.Actions, action)
	}
	return m
}

// cellsForRow builds one ingredient row's cells in grid order.
//
// A step cell belongs to the row where its span starts; every row the span
// covers emits nothing for that column. A column that holds no cell at this row
// but has cells to its right still has to be occupied, or the browser's implicit
// grid would shift the later cells one column left — so a run of empty columns
// becomes one classed gap cell (colspan), exactly like the blank "righthide"
// cells in the reference table. Trailing empty columns emit nothing: the row
// simply ends there.
func cellsForRow(spans []stepSpan, row int) []matrixCell {
	last := -1
	for col, span := range spans {
		if span.ok && span.first == row {
			last = col
		}
	}
	if last < 0 {
		return nil // only the label: every column is covered by a span or empty
	}

	var cells []matrixCell
	gap := 0
	for col := 0; col <= last; col++ {
		span := spans[col]
		switch {
		case span.ok && span.first == row:
			if gap > 0 {
				cells = append(cells, matrixCell{Kind: cellGap, Span: gap})
				gap = 0
			}
			cells = append(cells, matrixCell{Kind: cellStep, Verb: span.verb, Span: span.last - span.first + 1})
		case span.ok && row > span.first && row <= span.last:
			// Covered by the span of a step above: the row holds no cell here.
			if gap > 0 {
				cells = append(cells, matrixCell{Kind: cellGap, Span: gap})
				gap = 0
			}
		default:
			gap++
		}
	}
	return cells
}

// isIngredient reports whether an item is ingredient text rather than a
// back-reference. Empty text is not an ingredient.
func isIngredient(item Item) bool {
	return item.Ref <= 0 && item.Text != ""
}

// addsIngredient reports whether a step introduces ingredient text of its own.
// A step whose items are all references — or that has no items at all — adds
// nothing, and renders as a full-width action row (SPEC-ENGINE §4.1).
func addsIngredient(step Step) bool {
	for _, item := range step.Items {
		if isIngredient(item) {
			return true
		}
	}
	return false
}

// tableTemplate emits the matrix fragment. Literal markup lives here; every
// piece of user text is an action, so html/template escapes it in the right
// context. There is no stylesheet, no script and no toggle in the output.
//
// Column order is the document's order of steps, so the table reads left to
// right as time. The action rows come first, above the ingredient flow, and
// span every column.
var tableTemplate = template.Must(template.New("trn.table").Parse(`<table class="trn-table">
{{- if .Title}}<caption class="trn-title">{{.Title}}</caption>{{end}}
<thead>
<tr class="trn-head">
<th scope="col" class="trn-corner">Ingredient</th>
{{- range .Steps}}<th scope="col" class="trn-step trn-step-head"><span class="trn-step-num">[{{.Number}}]</span> <span class="trn-step-verb">{{.Verb}}</span></th>{{end}}
</tr>
</thead>
<tbody>
{{- range .Actions}}<tr class="trn-action">
<td class="trn-action-cell" colspan="{{$.ColSpan}}"><span class="trn-action-num">[{{.Number}}]</span> <span class="trn-action-verb">{{.Verb}}</span>{{if .Refs}} <span class="trn-action-refs">{{range .Refs}}<span class="trn-action-ref">{{if .Known}}[{{.Number}}] {{.Verb}}{{else}}#{{.Number}}{{end}}</span> {{end}}</span>{{end}}</td>
</tr>{{end}}
{{- range .Rows}}<tr class="trn-row">
<th scope="row" class="trn-ingredient">{{.Text}}</th>
{{- range .Cells}}{{if eq .Kind "step"}}<td class="trn-step"{{if gt .Span 1}} rowspan="{{.Span}}"{{end}}><span class="trn-step-verb">{{.Verb}}</span></td>{{else}}<td class="trn-gap"{{if gt .Span 1}} colspan="{{.Span}}"{{end}}></td>{{end}}{{end}}
</tr>{{end}}
</tbody>
</table>
`))
