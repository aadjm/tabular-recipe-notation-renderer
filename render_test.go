package trn

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The tests in this file read the rendered HTML back rather than trusting the
// model: SPEC-ENGINE E-A2 asks for counts and spans taken from the output, so
// every assertion below goes through parseTable, which reconstructs the HTML
// table grid (rowspan/colspan and all) the way a browser lays it out.

var (
	tableRowPattern     = regexp.MustCompile(`(?s)<tr\b([^>]*)>(.*?)</tr>`)
	tableCellPattern    = regexp.MustCompile(`(?s)<(th|td)\b([^>]*)>(.*?)</(?:th|td)>`)
	tableAttrPattern    = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
	tableMarkupPattern  = regexp.MustCompile(`<[^>]*>`)
	tableCaptionPattern = regexp.MustCompile(`(?s)<caption\b[^>]*>(.*?)</caption>`)
)

// parsedCell is one cell of the rendered table at its grid position. col is
// 1-based: column 1 is the ingredient label column, column 2 is the first step.
type parsedCell struct {
	row, col         int
	tag              string // "th" or "td"
	class            string
	rowspan, colspan int    // 1 when the attribute is absent
	text             string // rendered text, tags stripped and entities decoded
}

type parsedRow struct {
	index int // row index in the whole table, header row included
	class string
	cells []parsedCell
}

type parsedTable struct {
	caption string
	cols    int // grid columns held by the header row
	rows    []parsedRow
}

// parseTable reads the fragment back into a grid. Rowspans from earlier rows
// are honoured, so a cell's col is the column a browser would place it in —
// which is exactly what breaks when a renderer forgets a gap cell.
func parseTable(t *testing.T, fragment string) parsedTable {
	t.Helper()
	var tbl parsedTable
	if m := tableCaptionPattern.FindStringSubmatch(fragment); m != nil {
		tbl.caption = textOf(m[1])
	}
	occupied := make(map[[2]int]bool, 32)
	for i, rowMatch := range tableRowPattern.FindAllStringSubmatch(fragment, -1) {
		row := parsedRow{index: i, class: attrOf(rowMatch[1], "class")}
		cursor := 1
		for _, cellMatch := range tableCellPattern.FindAllStringSubmatch(rowMatch[2], -1) {
			for occupied[[2]int{i, cursor}] {
				cursor++
			}
			cell := parsedCell{
				row:     i,
				col:     cursor,
				tag:     cellMatch[1],
				class:   attrOf(cellMatch[2], "class"),
				rowspan: intAttr(t, cellMatch[2], "rowspan"),
				colspan: intAttr(t, cellMatch[2], "colspan"),
				text:    textOf(cellMatch[3]),
			}
			for dr := 0; dr < cell.rowspan; dr++ {
				for dc := 0; dc < cell.colspan; dc++ {
					occupied[[2]int{i + dr, cursor + dc}] = true
				}
			}
			row.cells = append(row.cells, cell)
			cursor += cell.colspan
		}
		if i == 0 {
			tbl.cols = cursor - 1
		}
		tbl.rows = append(tbl.rows, row)
	}
	if tbl.cols == 0 {
		t.Fatalf("no header row in fragment:\n%s", fragment)
	}
	return tbl
}

// intAttr reads an integer attribute, defaulting to 1 when it is absent.
func intAttr(t *testing.T, attrs, name string) int {
	t.Helper()
	raw := attrOf(attrs, name)
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		t.Fatalf("attribute %s=%q is not a positive integer", name, raw)
	}
	return n
}

func attrOf(attrs, name string) string {
	for _, m := range tableAttrPattern.FindAllStringSubmatch(attrs, -1) {
		if m[1] == name {
			return html.UnescapeString(m[2])
		}
	}
	return ""
}

func textOf(inner string) string {
	return strings.TrimSpace(html.UnescapeString(tableMarkupPattern.ReplaceAllString(inner, "")))
}

// at returns the cell occupying a grid position, if any.
func (tbl parsedTable) at(row, col int) (parsedCell, bool) {
	for _, r := range tbl.rows {
		for _, c := range r.cells {
			if row >= c.row && row < c.row+c.rowspan && col >= c.col && col < c.col+c.colspan {
				return c, true
			}
		}
	}
	return parsedCell{}, false
}

// row returns the row with the given table index.
func (tbl parsedTable) row(index int) parsedRow {
	for _, r := range tbl.rows {
		if r.index == index {
			return r
		}
	}
	var empty parsedRow
	return empty
}

func loadRecipe(t *testing.T, name string) *Recipe {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	r, warnings := Parse(string(src))
	if len(warnings) != 0 {
		t.Fatalf("fixture %s parsed with warnings %v; canonical fixtures must be clean", name, warnings)
	}
	return r
}

// ingredientRows returns the matrix' ingredient rows (class trn-row) and the
// full-width action rows (class trn-action) separately, in document order.
func ingredientRows(tbl parsedTable) (ingredients, actions []parsedRow) {
	for _, row := range tbl.rows {
		switch {
		case strings.Contains(row.class, "trn-row"):
			ingredients = append(ingredients, row)
		case strings.Contains(row.class, "trn-action"):
			actions = append(actions, row)
		}
	}
	return ingredients, actions
}

// label returns an ingredient row's label cell text, after checking that it is
// the row header of column 1.
func label(t *testing.T, row parsedRow) string {
	t.Helper()
	if len(row.cells) == 0 {
		t.Fatalf("ingredient row %d has no cells", row.index)
	}
	first := row.cells[0]
	if first.tag != "th" || first.col != 1 {
		t.Fatalf("row %d: label cell is <%s> at column %d, want <th> at column 1", row.index, first.tag, first.col)
	}
	return first.text
}

// checkGridShape asserts that every ingredient row's cells sit side by side in
// the grid: each cell starts where the previous one ended, nothing overlaps an
// earlier rowspan (the parser would have shifted it, changing its column), and
// no row runs past the header's columns.
func checkGridShape(t *testing.T, tbl parsedTable) {
	t.Helper()
	for _, row := range tbl.rows {
		want := 1
		for _, c := range row.cells {
			if c.col != want {
				t.Errorf("row %d: cell %q lands at column %d, want %d (a gap cell is missing or oversized)", row.index, c.text, c.col, want)
			}
			want = c.col + c.colspan
		}
		if want-1 > tbl.cols {
			t.Errorf("row %d spans %d grid columns, header has %d", row.index, want-1, tbl.cols)
		}
	}
}

// TestChaiHeaderStructure pins the table's head and the pure-HTML contract of
// the fragment: a real <table>, scoped headers ([n] + verb per column, one row
// header per ingredient), and no styling or script of any kind.
func TestChaiHeaderStructure(t *testing.T) {
	fragment := string(RenderTable(loadRecipe(t, "chai.trn.md")))

	for _, bad := range []string{"<style", "<script", "style=", "<div", "<span class=\"trn-mark"} {
		if strings.Contains(fragment, bad) {
			t.Errorf("matrix fragment contains %q; the engine emits structure only", bad)
		}
	}
	if !strings.HasPrefix(fragment, `<table class="trn-table">`) {
		t.Errorf("fragment does not start with the classed table: %.60q", fragment)
	}

	tbl := parseTable(t, fragment)
	if tbl.cols != 6 {
		t.Fatalf("grid has %d columns, want 6 (ingredient label + 5 steps)", tbl.cols)
	}
	header := tbl.row(0)
	if len(header.cells) != 6 {
		t.Fatalf("header has %d cells, want 6", len(header.cells))
	}
	if header.cells[0].tag != "th" || header.cells[0].text != "Ingredient" {
		t.Errorf("corner header = <%s> %q, want <th> %q", header.cells[0].tag, header.cells[0].text, "Ingredient")
	}
	wantHeaders := []string{
		"[1] Crush or grind to a powder",
		"[2] Boil 3 min in 360ml water until the decoction is dark",
		"[3] Add and bring to a boil",
		"[4] Simmer 2 min until dark with a cream layer and no raw milk flavour",
		"[5] Pull 4-5 times from 1-1.5 ft, then strain into cups and serve",
	}
	for i, want := range wantHeaders {
		cell := header.cells[i+1]
		if cell.tag != "th" || cell.col != i+2 {
			t.Errorf("step header %d: <%s> at column %d, want <th> at column %d", i+1, cell.tag, cell.col, i+2)
		}
		if cell.rowspan != 1 || cell.colspan != 1 {
			t.Errorf("step header %d spans %dx%d, want a plain cell", i+1, cell.rowspan, cell.colspan)
		}
		if cell.text != want {
			t.Errorf("step header %d = %q, want %q", i+1, cell.text, want)
		}
	}
	if !strings.Contains(fragment, `<th scope="col"`) {
		t.Error("step headers are not scope=\"col\"")
	}
	if !strings.Contains(fragment, `<th scope="row" class="trn-ingredient">`) {
		t.Error("ingredient labels are not scope=\"row\" headers")
	}
}

// TestEA2ChaiMatrix is acceptance criterion E-A2 against the rendered HTML:
// 8 ingredient rows, 5 step columns, step 1's cell spanning 4 rows, step 2's 6,
// step 3's all 8, and steps 4 and 5 as two full-width action rows.
//
// SPEC-ENGINE §7 phrases E-A2 as "11 ingredient rows × 5 step columns". Eleven
// does not follow from §4.1 ("rows: unique ingredients, ordered by first use"):
// the chai document has 8 unique ingredients, and 11 is the number of items in
// steps 1–4 with references counted as items. §4.1 is the normative rendering
// rule, so the matrix has 8 ingredient rows; the discrepancy is recorded in the
// README and on the board rather than papered over by inventing three rows.
func TestEA2ChaiMatrix(t *testing.T) {
	r := loadRecipe(t, "chai.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)

	if tbl.caption != "Masala Chai" {
		t.Errorf("caption = %q, want %q", tbl.caption, "Masala Chai")
	}

	ingredients, actions := ingredientRows(tbl)

	// Action rows: steps 4 and 5 add no ingredient, so each is one cell across
	// every column, and they sit above the ingredient flow.
	if len(actions) != 2 {
		t.Fatalf("got %d action rows, want 2 (steps 4 and 5)", len(actions))
	}
	for i, action := range actions {
		if i > 0 && action.index < actions[i-1].index {
			t.Errorf("action rows are not in document order")
		}
		if len(action.cells) != 1 {
			t.Fatalf("action row %d has %d cells, want exactly 1", i, len(action.cells))
		}
		cell := action.cells[0]
		if cell.colspan != 6 {
			t.Errorf("action row %d colspan = %d, want 6 (all columns)", i, cell.colspan)
		}
		if cell.tag != "td" || !strings.Contains(cell.class, "trn-action-cell") {
			t.Errorf("action row %d cell = <%s class=%q>, want the action cell", i, cell.tag, cell.class)
		}
	}
	if actions[0].index > ingredients[0].index {
		t.Errorf("action rows (row %d) come after the ingredient flow (row %d); the reference format puts them above it",
			actions[0].index, ingredients[0].index)
	}
	wantActions := []struct {
		number int
		verb   string
		ref    string
	}{
		{4, "Simmer 2 min until dark with a cream layer and no raw milk flavour", "[3] Add and bring to a boil"},
		{5, "Pull 4-5 times from 1-1.5 ft, then strain into cups and serve", "[4] Simmer 2 min until dark with a cream layer and no raw milk flavour"},
	}
	for i, want := range wantActions {
		wantText := "[" + strconv.Itoa(want.number) + "] " + want.verb + " " + want.ref
		if got := actions[i].cells[0].text; got != wantText {
			t.Errorf("action row %d text = %q, want %q", i, got, wantText)
		}
	}

	// Ingredient rows: unique ingredients in first-use order, one per row.
	wantIngredients := []string{
		"4 green cardamoms", "2 cloves", "3g cinnamon", "2 peppercorns",
		"1/2 inch ginger crushed", "6g black tea", "240ml full-fat milk", "12g sugar",
	}
	if len(ingredients) != len(wantIngredients) {
		t.Fatalf("got %d ingredient rows, want %d", len(ingredients), len(wantIngredients))
	}
	for i, want := range wantIngredients {
		if got := label(t, ingredients[i]); got != want {
			t.Errorf("ingredient row %d label = %q, want %q", i, got, want)
		}
	}

	// The matrix itself: one cell per ingredient-bearing step, in the column of
	// that step, starting at the first row it acts on.
	first := ingredients[0].index
	wantSpans := []struct {
		col     int
		rowspan int
		verb    string
	}{
		{2, 4, r.Steps[0].Verb},
		{3, 6, r.Steps[1].Verb},
		{4, 8, r.Steps[2].Verb},
	}
	for _, want := range wantSpans {
		cell, ok := tbl.at(first, want.col)
		if !ok {
			t.Fatalf("no cell at (row %d, column %d), want step %d's cell", first, want.col, want.col-1)
		}
		if cell.rowspan != want.rowspan {
			t.Errorf("step %d cell rowspan = %d, want %d (the rows it acts on)", want.col-1, cell.rowspan, want.rowspan)
		}
		if cell.colspan != 1 {
			t.Errorf("step %d cell colspan = %d, want 1", want.col-1, cell.colspan)
		}
		if cell.text != want.verb {
			t.Errorf("step %d cell text = %q, want the step verb %q", want.col-1, cell.text, want.verb)
		}
		if cell.class != "trn-step" {
			t.Errorf("step %d cell class = %q, want trn-step", want.col-1, cell.class)
		}
	}
	// Steps 4 and 5 are action rows, so their columns stay empty in the flow.
	for _, col := range []int{5, 6} {
		if cell, ok := tbl.at(first, col); ok {
			t.Errorf("column %d holds %q, but step %d is an action row", col, cell.text, col-1)
		}
	}

	// Every one of the 8 rows is covered by the final ingredient-bearing step's
	// cell: the finished dish.
	dish, ok := tbl.at(first, 4)
	if !ok {
		t.Fatal("no finished-dish cell")
	}
	for _, row := range ingredients {
		if _, covered := tbl.at(row.index, 4); !covered {
			t.Errorf("row %q is not covered by the finished-dish cell (rowspan %d)", label(t, row), dish.rowspan)
		}
	}
}

// TestGuacamoleMatrix pins the continuation-line example: refs and ingredients
// interleaved into one step, every ingredient folding into the final cell, and
// gap cells holding the time axis open where a later step has no cell yet.
func TestGuacamoleMatrix(t *testing.T) {
	r := loadRecipe(t, "guacamole.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)

	if tbl.cols != 5 {
		t.Fatalf("grid has %d columns, want 5 (label + 4 steps)", tbl.cols)
	}
	ingredients, actions := ingredientRows(tbl)
	if len(actions) != 0 {
		t.Errorf("got %d action rows, want none: step 4 adds cilantro and salt", len(actions))
	}
	want := []string{
		"3 ripe avocados", "1/2 red onion", "1 jalapeno", "1 roma tomato",
		"1 lime", "1/4 cup chopped cilantro", "1 tsp salt",
	}
	if len(ingredients) != len(want) {
		t.Fatalf("got %d ingredient rows, want %d", len(ingredients), len(want))
	}
	for i, text := range want {
		if got := label(t, ingredients[i]); got != text {
			t.Errorf("row %d label = %q, want %q", i, got, text)
		}
	}

	first := ingredients[0].index
	spans := []struct {
		row, col, rowspan int
	}{
		{first, 2, 1},         // [1] Mash: avocados
		{first + 1, 3, 3},     // [2] Finely dice: onion, jalapeno, tomato
		{first + 4, 4, 1},     // [3] Juice: lime
		{first, 5, len(want)}, // [4] Fold together: the whole flow
	}
	for _, want := range spans {
		cell, ok := tbl.at(want.row, want.col)
		if !ok {
			t.Fatalf("no cell at (row %d, column %d)", want.row, want.col)
		}
		if cell.rowspan != want.rowspan {
			t.Errorf("column %d cell at row %d: rowspan = %d, want %d", want.col, want.row, cell.rowspan, want.rowspan)
		}
		if wantVerb := r.Steps[want.col-2].Verb; cell.text != wantVerb {
			t.Errorf("column %d cell text = %q, want %q", want.col, cell.text, wantVerb)
		}
	}
	// Cells before the step cell that starts later in the row keep the columns
	// aligned: row 1 needs a two-column gap for steps 2 and 3.
	if cell, ok := tbl.at(first, 3); ok {
		if !strings.Contains(cell.class, "trn-gap") || cell.colspan != 2 {
			t.Errorf("first row gap = <%s class=%q colspan=%d>, want an empty two-column trn-gap", cell.tag, cell.class, cell.colspan)
		}
		if cell.text != "" {
			t.Errorf("gap cell carries text %q; it is structural only", cell.text)
		}
	} else {
		t.Error("first row has no gap cell for steps 2 and 3; the step cells cannot land in their columns")
	}
}

// TestMatrixMatchesReachability re-derives every cell from the AST with an
// independent walk for every fixture: the cell in a step's column starts at the
// first row the step acts on, spans to the last, and shows that step's verb.
// It also checks that the matrix agrees with the W4 channel: a stranded
// ingredient's row is not covered by the finished dish.
func TestMatrixMatchesReachability(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			r := loadRecipe(t, name)
			tbl := parseTable(t, string(RenderTable(r)))
			checkGridShape(t, tbl)
			ingredients, _ := ingredientRows(tbl)

			// Distinct ingredients in first-use order, with the rows that list
			// them.
			var order []string
			rowOf := map[string]int{}
			listedIn := map[string][]int{}
			for i, step := range r.Steps {
				for _, item := range step.Items {
					if item.Ref != 0 || item.Text == "" {
						continue
					}
					if _, seen := rowOf[item.Text]; !seen {
						rowOf[item.Text] = len(order)
						order = append(order, item.Text)
					}
					listedIn[item.Text] = append(listedIn[item.Text], i)
				}
			}
			if len(ingredients) != len(order) {
				t.Fatalf("got %d ingredient rows, want one per unique ingredient (%d)", len(ingredients), len(order))
			}
			firstRow := ingredients[0].index
			for i, text := range order {
				if got := label(t, ingredients[i]); got != text {
					t.Errorf("row %d label = %q, want %q (first-use order)", i, got, text)
				}
			}

			firstAt := map[int]int{}
			for i, step := range r.Steps {
				if _, seen := firstAt[step.Number]; !seen {
					firstAt[step.Number] = i
				}
			}

			for col, step := range r.Steps {
				acted := map[int]bool{}
				for i := range r.Steps {
					if !testReach(r.Steps, firstAt, col)[i] {
						continue
					}
					for _, item := range r.Steps[i].Items {
						if item.Ref == 0 && item.Text != "" {
							acted[rowOf[item.Text]] = true
						}
					}
				}

				// The cells of this column that carry a step (gap cells are
				// structural and may sit anywhere in the column).
				var stepCells []parsedCell
				for _, row := range ingredients {
					for _, c := range row.cells {
						if c.col == col+2 && strings.Contains(c.class, "trn-step") {
							stepCells = append(stepCells, c)
						}
					}
				}

				if !addsIngredient(step) || len(acted) == 0 {
					if len(stepCells) != 0 {
						t.Errorf("step %d adds no ingredient but has %d cells in the flow", step.Number, len(stepCells))
					}
					continue
				}
				if len(stepCells) != 1 {
					t.Fatalf("step %d has %d cells in its column, want exactly 1", step.Number, len(stepCells))
				}
				lo, hi := len(order), -1
				for row := range acted {
					if row < lo {
						lo = row
					}
					if row > hi {
						hi = row
					}
				}
				cell := stepCells[0]
				if want := firstRow + lo; cell.row != want {
					t.Errorf("step %d cell starts at row %d, want the first row it acts on (%d)", step.Number, cell.row, want)
				}
				if want := hi - lo + 1; cell.rowspan != want {
					t.Errorf("step %d rowspan = %d, want %d (rows %d…%d)", step.Number, cell.rowspan, want, lo, hi)
				}
				if cell.text != step.Verb {
					t.Errorf("step %d cell text = %q, want the verb %q", step.Number, cell.text, step.Verb)
				}
			}

			// Agreement with the W4 channel.
			stranded := testStranded(r)
			warnings := ConvergenceWarnings(r)
			if len(warnings) != len(stranded) {
				t.Fatalf("ConvergenceWarnings reported %d ingredients, matrix-derived count is %d", len(warnings), len(stranded))
			}
			finalCol := len(r.Steps) + 1
			for i, text := range stranded {
				if warnings[i].Code != "W4" || !strings.Contains(warnings[i].Msg, strconv.Quote(text)) {
					t.Errorf("warning %d = %+v, want W4 naming %q", i, warnings[i], text)
				}
				row := firstRow + rowOf[text]
				if cell, ok := tbl.at(row, finalCol); ok && strings.Contains(cell.class, "trn-step") {
					t.Errorf("stranded ingredient %q is covered by the finished-dish cell %q", text, cell.text)
				}
			}
		})
	}
}

func lists(step Step, text string) bool {
	for _, item := range step.Items {
		if item.Ref == 0 && item.Text == text {
			return true
		}
	}
	return false
}

func testReach(steps []Step, firstAt map[int]int, target int) map[int]bool {
	out := map[int]bool{}
	var walk func(int)
	walk = func(i int) {
		if out[i] {
			return
		}
		out[i] = true
		for _, item := range steps[i].Items {
			if item.Ref <= 0 {
				continue
			}
			if j, ok := firstAt[item.Ref]; ok {
				walk(j)
			}
		}
	}
	walk(target)
	return out
}

// testStranded lists the ingredients that do not reach the last step, in
// first-use order — the expectation ConvergenceWarnings must match.
func testStranded(r *Recipe) []string {
	if len(r.Steps) == 0 {
		return nil
	}
	firstAt := map[int]int{}
	for i, step := range r.Steps {
		if _, seen := firstAt[step.Number]; !seen {
			firstAt[step.Number] = i
		}
	}
	reaches := testReach(r.Steps, firstAt, len(r.Steps)-1)
	var order []string
	seen := map[string]bool{}
	for _, step := range r.Steps {
		for _, item := range step.Items {
			if item.Ref != 0 || item.Text == "" || seen[item.Text] {
				continue
			}
			seen[item.Text] = true
			ok := false
			for i := range reaches {
				if reaches[i] && i < len(r.Steps) && lists(r.Steps[i], item.Text) {
					ok = true
					break
				}
			}
			if !ok {
				order = append(order, item.Text)
			}
		}
	}
	return order
}

// TestRenderTableEscaping pushes markup, quotes and Unicode through every text
// field and reads it back: user bytes survive intact (only HTML-escaped), and
// nothing user-supplied turns into live markup.
func TestRenderTableEscaping(t *testing.T) {
	const (
		title = `Chai & "match" </caption><script>alert(1)</script> — ’tis`
		verb1 = `Chop <b>"onion"</b> & 'garlic'`
		verb2 = `Wait <until> done`
		ing1  = `1/2 <cup> & "oil" — ’olive’`
		ing2  = `1 tsp <salt> & pepper`
	)
	r := &Recipe{
		Title: title,
		Steps: []Step{
			{Number: 1, Verb: verb1, Items: []Item{{Text: ing1}}},
			{Number: 2, Verb: `Fold <i>in</i>`, Items: []Item{{Text: ing2}}},
			{Number: 3, Verb: verb2, Items: []Item{{Ref: 2}, {Text: `<style>p{}</style>`}}},
		},
	}
	out := string(RenderTable(r))
	for _, bad := range []string{"<script", "<b>", "<i>", "<img", "<svg", "<style"} {
		if strings.Contains(out, bad) {
			t.Errorf("rendered HTML contains live markup %q from user text", bad)
		}
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the title's <script> to appear HTML-escaped")
	}
	if strings.Contains(out, "<style") {
		t.Error("user text produced a style element")
	}

	tbl := parseTable(t, out)
	ingredients, actions := ingredientRows(tbl)
	if tbl.caption != title {
		t.Errorf("caption round-trip = %q, want %q", tbl.caption, title)
	}
	if len(ingredients) != 3 {
		t.Fatalf("got %d ingredient rows, want 3", len(ingredients))
	}
	for i, want := range []string{ing1, ing2, `<style>p{}</style>`} {
		if got := label(t, ingredients[i]); got != want {
			t.Errorf("ingredient %d round-trip = %q, want %q", i, got, want)
		}
	}
	if got := tbl.row(0).cells[1].text; got != "[1] "+verb1 {
		t.Errorf("step header round-trip = %q, want %q", got, "[1] "+verb1)
	}
	if got := tbl.row(0).cells[2].text; got != "[2] Fold <i>in</i>" {
		t.Errorf("step header round-trip = %q, want %q", got, "[2] Fold <i>in</i>")
	}
	if len(actions) != 0 {
		t.Fatalf("got %d action rows, want none (every step adds an ingredient)", len(actions))
	}
	// Step 3 references step 2 and adds an ingredient of its own, so its cell
	// starts at step 2's row (the "1 tsp <salt> & pepper" row, row 1 in the
	// flow) and spans through its own ingredient: ingredient 1 never reaches
	// step 3, which is W4's business, not the table's.
	cell, ok := tbl.at(ingredients[1].index, 4)
	if !ok {
		t.Fatal("no step 3 cell at the second ingredient's row")
	}
	if cell.text != verb2 {
		t.Errorf("step 3 cell text = %q, want the verb %q", cell.text, verb2)
	}
	if cell.rowspan != 2 {
		t.Errorf("step 3 rowspan = %d, want 2 (step 2's ingredient and its own)", cell.rowspan)
	}
}

// TestRenderTablePureHTML is the structural half of SPEC-ENGINE §10: the matrix
// fragment is semantic HTML only — nothing executable, nothing styled, nothing
// fetched, no view toggle.
func TestRenderTablePureHTML(t *testing.T) {
	for _, name := range fixtureNames(t) {
		out := string(RenderTable(loadRecipe(t, name)))
		for _, bad := range []string{
			"<style", "</style", "<script", "<link", "<img", "<svg", "<canvas",
			"src=", "javascript:", "@import", "url(", "http://", "https://",
			"display:", "class=\"trn-view", "<input", "<label",
		} {
			if strings.Contains(out, bad) {
				t.Errorf("%s: rendered HTML contains %q", name, bad)
			}
		}
		if n := strings.Count(out, "<table"); n != 1 {
			t.Errorf("%s: fragment has %d tables, want exactly 1", name, n)
		}
	}
}

// TestRenderTableNilAndEmpty covers the degenerate documents: no input, no
// steps, no title. None of them may panic, all of them must stay well-formed.
func TestRenderTableNilAndEmpty(t *testing.T) {
	if got := RenderTable(nil); got != "" {
		t.Errorf("RenderTable(nil) = %q, want empty fragment", got)
	}

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"no steps", "Just a title"},
		{"steps, no title", "[1] Stir: water"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := Parse(tc.input)
			out := string(RenderTable(r))
			if !strings.Contains(out, `<table class="trn-table">`) {
				t.Errorf("no table element in output:\n%s", out)
			}
			if strings.Contains(out, `class="trn-action"`) {
				t.Error("document without reference-only steps produced an action row")
			}
			if strings.Count(out, "</table>") != 1 {
				t.Errorf("expected exactly one table, got %d", strings.Count(out, "</table>"))
			}
			if strings.Contains(out, "<caption") != (r.Title != "") {
				t.Errorf("caption presence = %v, want %v (title %q)", strings.Contains(out, "<caption"), r.Title != "", r.Title)
			}
		})
	}
}

// TestRenderTableDegenerateSteps pins the less obvious structural choices: a
// step with no items at all is an action row, a dangling reference renders by
// number, a duplicated step number resolves to its first appearance, and a span
// swallows a stranded ingredient sitting between two rows the step acts on.
func TestRenderTableDegenerateSteps(t *testing.T) {
	r := &Recipe{
		Title: "T",
		Steps: []Step{
			{Number: 1, Verb: "Mash", Items: []Item{{Text: "a"}}},
			{Number: 2, Verb: "Rest"}, // no items: adds nothing
			{Number: 3, Verb: "Combine", Items: []Item{{Ref: 1}, {Ref: 2}, {Ref: 9}}},
		},
	}
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)
	ingredients, actions := ingredientRows(tbl)
	if len(ingredients) != 1 || len(actions) != 2 {
		t.Fatalf("got %d ingredient rows and %d action rows, want 1 and 2", len(ingredients), len(actions))
	}
	if got, want := actions[0].cells[0].text, "[2] Rest"; got != want {
		t.Errorf("step with no items = %q, want %q", got, want)
	}
	if got, want := actions[1].cells[0].text, "[3] Combine [1] Mash [2] Rest #9"; got != want {
		t.Errorf("action refs = %q, want %q", got, want)
	}

	// Duplicated numbers: #1 resolves to the first step numbered 1.
	dup := &Recipe{Steps: []Step{
		{Number: 1, Verb: "A", Items: []Item{{Text: "x"}}},
		{Number: 1, Verb: "B", Items: []Item{{Ref: 1}}},
	}}
	dupTbl := parseTable(t, string(RenderTable(dup)))
	dupIngredients, dupActions := ingredientRows(dupTbl)
	if len(dupActions) != 1 {
		t.Fatalf("got %d action rows, want 1", len(dupActions))
	}
	if got, want := dupActions[0].cells[0].text, "[1] B [1] A"; got != want {
		t.Errorf("duplicate-number ref = %q, want %q (first appearance)", got, want)
	}
	if cell, ok := dupTbl.at(dupIngredients[0].index, 2); !ok || cell.text != "A" {
		t.Errorf("duplicate-number matrix row = %+v, want the first step's cell", cell)
	}
	if cell, ok := dupTbl.at(dupIngredients[0].index, 3); ok && strings.Contains(cell.class, "trn-step") {
		t.Errorf("the reference-only step has a cell %q in the flow, want an action row only", cell.text)
	}

	// A stranded ingredient between two rows a step acts on is swallowed by the
	// span: an HTML rowspan is contiguous, and W4 already reports the
	// ingredient. C4E merging: what one cell covers is one thing being worked on.
	stranded := &Recipe{Steps: []Step{
		{Number: 1, Verb: "Slice", Items: []Item{{Text: "onion"}}},
		{Number: 2, Verb: "Grate", Items: []Item{{Text: "cheese"}}},
		{Number: 3, Verb: "Top", Items: []Item{{Ref: 1}, {Text: "base"}}},
	}}
	strandedTbl := parseTable(t, string(RenderTable(stranded)))
	strandedIngredients, _ := ingredientRows(strandedTbl)
	if len(strandedIngredients) != 3 {
		t.Fatalf("got %d ingredient rows, want 3", len(strandedIngredients))
	}
	cell, ok := strandedTbl.at(strandedIngredients[0].index, 4)
	if !ok {
		t.Fatal("step 3 has no cell")
	}
	if cell.rowspan != 3 {
		t.Errorf("step 3 rowspan = %d, want 3 (rows onion…base, the stranded cheese included)", cell.rowspan)
	}
	warnings := ConvergenceWarnings(stranded)
	if len(warnings) != 1 || !strings.Contains(warnings[0].Msg, `"cheese"`) {
		t.Errorf("stranded ingredient not reported: %+v", warnings)
	}
}

// fixtureNames lists the committed *.trn.md fixtures by base name.
func fixtureNames(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("testdata/*.trn.md")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no fixtures found under testdata/")
	}
	names := make([]string, 0, len(matches))
	for _, path := range matches {
		names = append(names, strings.TrimPrefix(path, "testdata/"))
	}
	return names
}
