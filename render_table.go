package trn

import (
	"bytes"
	"html/template"
	"sort"
)

// RenderTable renders a recipe as the Cooking-for-Engineers recipe summary
// table (SPEC-ENGINE §4.1): a semantic <table> in which every unique
// ingredient is a row, every step is placed in the earliest available depth
// column to the right of its inputs, and each step is ONE cell that spans —
// with rowspan — the ingredient rows it acts on: the ingredients it lists plus
// every ingredient carried into it through a back-reference chain.
//
// Steps that act on no ingredients (no ingredient items and no references to
// ingredient-bearing steps) render as full-width prep/action rows above the
// ingredient flow — the "preheat the oven" band of the reference format.
// Steps that reference an earlier mixture continue the flow as a step cell
// spanning the rows of that mixture (such as simmer, drain, or bake).
//
// Parallel steps that act on disjoint sets of rows share the same depth column
// (like "Mash" on row 1, "Finely dice" on rows 2–4, and "Juice" on row 5 in
// Guacamole, or "Grate" on row 5 in Margherita Pizza).
//
// The output is structure only: no <style>, no <script>, no view toggle, no
// external resource. Styling and the view switcher belong to the embedding
// site (SPEC-ENGINE §4.3). Every user string goes through html/template's
// escaping paths, so ingredient text keeps its bytes and differs only by HTML
// escaping (SPEC-ENGINE §10).
//
// RenderTable never panics and never errors: a nil recipe renders nothing, a
// document without steps renders an empty table.
// RenderTable renders a recipe as the Cooking-for-Engineers recipe summary
// table with the title hidden by default. Use RenderTableWithTitle(r, true)
// or RenderWithOptions to include a visible <caption> title.
func RenderTable(r *Recipe) template.HTML {
	return RenderTableWithTitle(r, false)
}

// RenderTableWithTitle renders the recipe summary table, optionally including
// the recipe title as a <caption> when showTitle is true.
func RenderTableWithTitle(r *Recipe, showTitle bool) template.HTML {
	if r == nil {
		return ""
	}
	m := buildMatrix(r)
	if !showTitle {
		m.Title = ""
	}
	var buf bytes.Buffer
	if err := tableTemplate.Execute(&buf, m); err != nil {
		return ""
	}
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
	Rows    []matrixRow    // unique ingredients, first-appearance order
	Actions []matrixAction // prep steps that act on no ingredient rows, document order
	ColSpan int            // ingredient label column + all depth columns
}

// matrixCell is one cell of an ingredient row, in grid order. A step cell
// carries the step's verb and the rowspan it was given; a gap cell carries the
// rowspan and colspan of the empty grid region it holds open.
type matrixCell struct {
	Kind    string // cellStep or cellGap
	Number  int    // step number, only for cellStep
	Verb    string // the step's verb, only for cellStep
	RowSpan int    // rowspan (both cellStep and cellGap)
	ColSpan int    // colspan (only for cellGap)
}

type matrixRow struct {
	Text  string
	Cells []matrixCell // in column order, label column excluded
}

// matrixAction is a prep step that touches no ingredient rows (no ingredients
// of its own and no references to ingredient-bearing steps): it spans the full
// width of the table above the ingredient flow.
type matrixAction struct {
	Number int
	Verb   string
	Refs   []stepRef // any steps it names, in written order
}

// stepRef is one reference written in a step, resolved against the document.
// Known is false for a reference to a step that does not exist (already
// reported as W2); such a reference is shown by number only.
type stepRef struct {
	Number int
	Verb   string
	Known  bool
}

// stepPlacement is one ingredient-bearing step's span and dependency depth.
type stepPlacement struct {
	stepIndex int
	number    int
	verb      string
	first     int
	last      int
	depth     int
}

// buildMatrix derives the matrix model from the AST.
//
// Reachability comes from ancestorSteps (the same closure ConvergenceWarnings
// uses, so the matrix and W4 can never disagree about whether an ingredient
// reaches a step). Parallel steps with non-overlapping row spans are packed
// into the earliest depth column where all of their spanned rows are free and
// strictly to the right of any referenced step's column.
func buildMatrix(r *Recipe) matrixView {
	m := matrixView{
		Title: r.Title,
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

	firstAt := firstByNumber(r)
	stepDepth := make([]int, len(r.Steps))
	for i, step := range r.Steps {
		d := 0
		for _, item := range step.Items {
			if item.Ref <= 0 {
				continue
			}
			if j, ok := firstAt[item.Ref]; ok && j < i {
				if stepDepth[j]+1 > d {
					d = stepDepth[j] + 1
				}
			}
		}
		stepDepth[i] = d
	}

	var placements []stepPlacement
	for idx, step := range r.Steps {
		reach := ancestorSteps(r, idx)
		first, last := len(order), -1
		for i := range r.Steps {
			if i >= len(reach) || !reach[i] {
				continue
			}
			for _, item := range r.Steps[i].Items {
				if !isIngredient(item) {
					continue
				}
				row := rowOf[item.Text]
				if row < first {
					first = row
				}
				if row > last {
					last = row
				}
			}
		}
		if first <= last {
			placements = append(placements, stepPlacement{
				stepIndex: idx,
				number:    step.Number,
				verb:      step.Verb,
				first:     first,
				last:      last,
				depth:     stepDepth[idx],
			})
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

	// Sort placements by dependency depth (preserving document order within a depth)
	// and pack into columns so parallel steps share a column while steps that
	// combine earlier outputs sit strictly to the right of their inputs.
	sort.SliceStable(placements, func(i, j int) bool {
		return placements[i].depth < placements[j].depth
	})

	type colState struct {
		used  []bool
		steps []stepPlacement
	}
	var cols []colState
	colOfStep := make(map[int]int, len(placements))
	for _, p := range placements {
		minCol := 0
		for _, item := range r.Steps[p.stepIndex].Items {
			if item.Ref <= 0 {
				continue
			}
			if j, ok := firstAt[item.Ref]; ok {
				if c, placed := colOfStep[j]; placed && c+1 > minCol {
					minCol = c + 1
				}
			}
		}
		placedCol := -1
		for c := minCol; c < len(cols); c++ {
			free := true
			for row := p.first; row <= p.last; row++ {
				if cols[c].used[row] {
					free = false
					break
				}
			}
			if free {
				placedCol = c
				break
			}
		}
		if placedCol < 0 {
			for len(cols) <= minCol {
				cols = append(cols, colState{used: make([]bool, len(order))})
			}
			placedCol = len(cols) - 1
			free := true
			for row := p.first; row <= p.last; row++ {
				if cols[placedCol].used[row] {
					free = false
					break
				}
			}
			if !free {
				cols = append(cols, colState{used: make([]bool, len(order))})
				placedCol = len(cols) - 1
			}
		}
		for row := p.first; row <= p.last; row++ {
			cols[placedCol].used[row] = true
		}
		cols[placedCol].steps = append(cols[placedCol].steps, p)
		colOfStep[p.stepIndex] = placedCol
	}

	numCols := len(cols)
	m.ColSpan = 1 + numCols

	// Build a 2D cell grid: rows × numCols.
	type slotState int
	const (
		slotEmpty slotState = iota
		slotStart
		slotSkip
	)
	type gridSlot struct {
		state   slotState
		number  int
		verb    string
		rowspan int
	}
	grid := make([][]gridSlot, len(order))
	for row := range grid {
		grid[row] = make([]gridSlot, numCols)
	}
	for c, col := range cols {
		for _, p := range col.steps {
			grid[p.first][c] = gridSlot{
				state:   slotStart,
				number:  p.number,
				verb:    p.verb,
				rowspan: p.last - p.first + 1,
			}
			for row := p.first + 1; row <= p.last; row++ {
				grid[row][c] = gridSlot{state: slotSkip}
			}
		}
	}

	// Coalesce empty slots horizontally per row; only merge across rows (rowspan)
	// when a step in an earlier column on the same row also spans those rows,
	// or keep per-row horizontal colspan so each ingredient row retains its own
	// horizontal pass-through band into the step that consumes it.
	m.Rows = make([]matrixRow, len(order))
	for row, text := range order {
		var cells []matrixCell
		gap := 0
		for c := 0; c < numCols; c++ {
			slot := grid[row][c]
			switch slot.state {
			case slotStart:
				if gap > 0 {
					cells = append(cells, matrixCell{Kind: cellGap, RowSpan: 1, ColSpan: gap})
					gap = 0
				}
				cells = append(cells, matrixCell{
					Kind:    cellStep,
					Number:  slot.number,
					Verb:    slot.verb,
					RowSpan: slot.rowspan,
					ColSpan: 1,
				})
			case slotSkip:
				if gap > 0 {
					cells = append(cells, matrixCell{Kind: cellGap, RowSpan: 1, ColSpan: gap})
					gap = 0
				}
			case slotEmpty:
				gap++
			}
		}
		if gap > 0 {
			cells = append(cells, matrixCell{Kind: cellGap, RowSpan: 1, ColSpan: gap})
		}
		m.Rows[row] = matrixRow{Text: text, Cells: cells}
	}

	return m
}

// isIngredient reports whether an item is ingredient text rather than a
// back-reference. Empty text is not an ingredient.
func isIngredient(item Item) bool {
	return item.Ref <= 0 && item.Text != ""
}

// tableTemplate emits the matrix fragment. Literal markup lives here; every
// piece of user text is an action, so html/template escapes it in the right
// context. There is no stylesheet, no script and no toggle in the output.
//
// In the C4E / Tabular Recipe Notation format, there is no redundant column
// header row repeating all step instructions across the top; ingredients run
// down the left and operations nest to the right across depth columns.
var tableTemplate = template.Must(template.New("trn.table").Parse(`<table class="trn-table">
{{- if .Title}}<caption class="trn-title">{{.Title}}</caption>{{end}}
<tbody>
{{- range .Actions}}<tr class="trn-action">
<td class="trn-action-cell" colspan="{{$.ColSpan}}"><span class="trn-action-num">[{{.Number}}]</span> <span class="trn-action-verb">{{.Verb}}</span>{{if .Refs}} <span class="trn-action-refs">{{range .Refs}}<span class="trn-action-ref">{{if .Known}}[{{.Number}}] {{.Verb}}{{else}}#{{.Number}}{{end}}</span> {{end}}</span>{{end}}</td>
</tr>{{end}}
{{- range .Rows}}<tr class="trn-row">
<th scope="row" class="trn-ingredient">{{.Text}}</th>
{{- range .Cells}}{{if eq .Kind "step"}}<td class="trn-step"{{if gt .RowSpan 1}} rowspan="{{.RowSpan}}"{{end}}><span class="trn-step-verb">{{.Verb}}</span></td>{{else}}<td class="trn-gap"{{if gt .RowSpan 1}} rowspan="{{.RowSpan}}"{{end}}{{if gt .ColSpan 1}} colspan="{{.ColSpan}}"{{end}}></td>{{end}}{{end}}
</tr>{{end}}
</tbody>
</table>
`))
