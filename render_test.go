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
// model: every assertion below goes through parseTable, which reconstructs the
// HTML table grid (rowspan/colspan and all) the way a browser lays it out.

var (
	tableRowPattern     = regexp.MustCompile(`(?s)<tr\b([^>]*)>(.*?)</tr>`)
	tableCellPattern    = regexp.MustCompile(`(?s)<(th|td)\b([^>]*)>(.*?)</(?:th|td)>`)
	tableAttrPattern    = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
	tableMarkupPattern  = regexp.MustCompile(`<[^>]*>`)
	tableCaptionPattern = regexp.MustCompile(`(?s)<caption\b[^>]*>(.*?)</caption>`)
)

// parsedCell is one cell of the rendered table at its grid position. col is
// 1-based: column 1 is the ingredient label column, column 2 is the first depth
// column.
type parsedCell struct {
	row, col         int
	tag              string // "th" or "td"
	class            string
	rowspan, colspan int    // 1 when the attribute is absent
	text             string // rendered text, tags stripped and entities decoded
}

type parsedRow struct {
	index int // row index in the whole table
	class string
	cells []parsedCell
}

type parsedTable struct {
	caption string
	cols    int // grid columns held by the table
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

const (
	inputDir  = "examples/input"
	outputDir = "examples/output"
)

func loadRecipe(t *testing.T, name string) *Recipe {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(inputDir, name))
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

// checkGridShape asserts that every grid position (row, col) in the table is
// covered by exactly one cell (either starting at that row or spanning down
// from an earlier row), and that every row spans the exact same number of columns.
func checkGridShape(t *testing.T, tbl parsedTable) {
	t.Helper()
	for _, row := range tbl.rows {
		for col := 1; col <= tbl.cols; col++ {
			if _, ok := tbl.at(row.index, col); !ok {
				t.Errorf("row %d, col %d is uncovered (missing cell or gap)", row.index, col)
			}
		}
		for _, c := range row.cells {
			if c.col+c.colspan-1 > tbl.cols {
				t.Errorf("row %d cell %q extends to col %d, table has %d cols", row.index, c.text, c.col+c.colspan-1, tbl.cols)
			}
		}
	}
}

// TestChaiStructure pins the pure-HTML contract of the fragment: a real <table>,
// no redundant thead, scoped row headers for ingredients, and no styling or
// script of any kind.
func TestChaiStructure(t *testing.T) {
	fragment := string(RenderTable(loadRecipe(t, "chai.trn.md")))

	for _, bad := range []string{"<style", "<script", "style=", "<div", "<thead", "<span class=\"trn-mark"} {
		if strings.Contains(fragment, bad) {
			t.Errorf("matrix fragment contains %q; the engine emits structure only", bad)
		}
	}
	if !strings.HasPrefix(fragment, `<table class="trn-table">`) {
		t.Errorf("fragment does not start with the classed table: %.60q", fragment)
	}
	if !strings.Contains(fragment, `<th scope="row" class="trn-ingredient">`) {
		t.Error("ingredient labels are not scope=\"row\" headers")
	}

	tbl := parseTable(t, fragment)
	if tbl.cols != 6 {
		t.Fatalf("grid has %d columns, want 6 (ingredient label + 5 depth columns)", tbl.cols)
	}
}

// TestEA2ChaiMatrix verifies the rendered HTML for the Masala Chai recipe:
// 8 ingredient rows, 5 step columns, step 1's cell spanning 4 rows, step 2's 6,
// steps 3, 4, and 5 spanning all 8 rows in their respective columns (since steps
// 4 and 5 act on the mixture from step 3), and zero prep/action rows.
func TestEA2ChaiMatrix(t *testing.T) {
	r := loadRecipe(t, "chai.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)

	if tbl.caption != "" {
		t.Errorf("default caption = %q, want empty (title hidden by default)", tbl.caption)
	}
	titledTbl := parseTable(t, string(RenderTableWithTitle(r, true)))
	if titledTbl.caption != "Masala Chai" {
		t.Errorf("titled caption = %q, want %q", titledTbl.caption, "Masala Chai")
	}

	ingredients, actions := ingredientRows(tbl)

	if len(actions) != 0 {
		t.Fatalf("got %d action rows, want 0 (all steps act on the ingredient flow)", len(actions))
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

	first := ingredients[0].index
	wantSpans := []struct {
		col     int
		rowspan int
		verb    string
	}{
		{2, 4, r.Steps[0].Verb},
		{3, 6, r.Steps[1].Verb},
		{4, 8, r.Steps[2].Verb},
		{5, 8, r.Steps[3].Verb},
		{6, 8, r.Steps[4].Verb},
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

	// Every one of the 8 rows is covered by the final step's cell: the finished dish.
	dish, ok := tbl.at(first, 6)
	if !ok {
		t.Fatal("no finished-dish cell")
	}
	for _, row := range ingredients {
		if _, covered := tbl.at(row.index, 6); !covered {
			t.Errorf("row %q is not covered by the finished-dish cell (rowspan %d)", label(t, row), dish.rowspan)
		}
	}
}

// TestGuacamoleMatrix pins the continuation-line and parallel-step example:
// steps 1, 2, and 3 are independent (depth 0) and share column 2 across their
// respective rows, while step 4 (depth 1) sits in column 3 and spans all 7 rows.
func TestGuacamoleMatrix(t *testing.T) {
	r := loadRecipe(t, "guacamole.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)

	if tbl.cols != 3 {
		t.Fatalf("grid has %d columns, want 3 (label + 2 depth columns)", tbl.cols)
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
		verb              string
	}{
		{first, 2, 1, r.Steps[0].Verb},         // [1] Mash: avocados (col 2)
		{first + 1, 2, 3, r.Steps[1].Verb},     // [2] Finely dice: onion, jalapeno, tomato (col 2)
		{first + 4, 2, 1, r.Steps[2].Verb},     // [3] Juice: lime (col 2)
		{first, 3, len(want), r.Steps[3].Verb}, // [4] Fold together: all 7 rows (col 3)
	}
	for _, want := range spans {
		cell, ok := tbl.at(want.row, want.col)
		if !ok {
			t.Fatalf("no cell at (row %d, column %d)", want.row, want.col)
		}
		if cell.row != want.row {
			t.Errorf("cell %q starts at row %d, want %d", want.verb, cell.row, want.row)
		}
		if cell.rowspan != want.rowspan {
			t.Errorf("cell %q at (row %d, col %d): rowspan = %d, want %d", want.verb, want.row, want.col, cell.rowspan, want.rowspan)
		}
		if cell.text != want.verb {
			t.Errorf("cell at (row %d, col %d) text = %q, want %q", want.row, want.col, cell.text, want.verb)
		}
	}
	// Rows 5 and 6 (cilantro and salt) have gap cells in column 2 before joining Fold together in column 3.
	for _, rIdx := range []int{first + 5, first + 6} {
		cell, ok := tbl.at(rIdx, 2)
		if !ok || !strings.Contains(cell.class, "trn-gap") {
			t.Errorf("row %d col 2 = %+v, want trn-gap", rIdx, cell)
		}
	}
}

// TestMatrixMatchesReachability re-derives every cell from the AST with an
// independent walk for every fixture: each ingredient-bearing step appears once
// in the table starting at the first row it acts on, spanning to the last, and
// placed to the right of any step it references.
func TestMatrixMatchesReachability(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			r := loadRecipe(t, name)
			tbl := parseTable(t, string(RenderTable(r)))
			checkGridShape(t, tbl)
			ingredients, _ := ingredientRows(tbl)

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

			var allStepCells []parsedCell
			for _, row := range ingredients {
				for _, c := range row.cells {
					if strings.Contains(c.class, "trn-step") {
						allStepCells = append(allStepCells, c)
					}
				}
			}

			wantStepCells := 0
			for idx, step := range r.Steps {
				acted := map[int]bool{}
				for i := range r.Steps {
					if !testReach(r.Steps, firstAt, idx)[i] {
						continue
					}
					for _, item := range r.Steps[i].Items {
						if item.Ref == 0 && item.Text != "" {
							acted[rowOf[item.Text]] = true
						}
					}
				}
				if len(acted) == 0 {
					continue
				}
				wantStepCells++
				lo, hi := len(order), -1
				for row := range acted {
					if row < lo {
						lo = row
					}
					if row > hi {
						hi = row
					}
				}
				found := false
				for _, c := range allStepCells {
					if c.text == step.Verb && c.row == firstRow+lo && c.rowspan == hi-lo+1 {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("step [%d] %q: no matching cell starting at row %d with rowspan %d", step.Number, step.Verb, firstRow+lo, hi-lo+1)
				}
			}
			if len(allStepCells) != wantStepCells {
				t.Errorf("got %d step cells in table, want %d", len(allStepCells), wantStepCells)
			}

			// Agreement with the W4 channel.
			stranded := testStranded(r)
			warnings := ConvergenceWarnings(r)
			if len(warnings) != len(stranded) {
				t.Fatalf("ConvergenceWarnings reported %d ingredients, matrix-derived count is %d", len(warnings), len(stranded))
			}
			finalCol := tbl.cols
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
	out := string(RenderTableWithTitle(r, true))
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
	if len(actions) != 0 {
		t.Fatalf("got %d action rows, want none (every step adds an ingredient)", len(actions))
	}
	// Step 1 and Step 2 are parallel (depth 0) so both sit in column 2 (rows 0 and 1).
	// Step 3 references Step 2 (depth 1), so it sits in column 3 starting at row 1
	// and spanning 2 rows (step 2's ingredient and its own).
	if cell, ok := tbl.at(ingredients[0].index, 2); !ok || cell.text != verb1 {
		t.Errorf("step 1 cell = %+v, want %q", cell, verb1)
	}
	if cell, ok := tbl.at(ingredients[1].index, 2); !ok || cell.text != `Fold <i>in</i>` {
		t.Errorf("step 2 cell = %+v, want %q", cell, `Fold <i>in</i>`)
	}
	cell, ok := tbl.at(ingredients[1].index, 3)
	if !ok {
		t.Fatal("no step 3 cell at the second ingredient's row in column 3")
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
			if strings.Contains(out, "<caption") {
				t.Errorf("default RenderTable should hide caption, got:\n%s", out)
			}
			outTitled := string(RenderTableWithTitle(r, true))
			if strings.Contains(outTitled, "<caption") != (r.Title != "") {
				t.Errorf("titled caption presence = %v, want %v (title %q)", strings.Contains(outTitled, "<caption"), r.Title != "", r.Title)
			}
		})
	}
}

// TestRenderTableDegenerateSteps pins the less obvious structural choices: a
// step that acts on no ingredients is an action row, a dangling reference
// renders by number, a duplicated step number resolves to its first appearance,
// and a span swallows a stranded ingredient sitting between two rows the step
// acts on.
func TestRenderTableDegenerateSteps(t *testing.T) {
	r := &Recipe{
		Title: "T",
		Steps: []Step{
			{Number: 1, Verb: "Preheat"},                                 // no items: acts on no ingredients
			{Number: 2, Verb: "Wait", Items: []Item{{Ref: 1}, {Ref: 9}}}, // refs only non-ingredient steps
			{Number: 3, Verb: "Mash", Items: []Item{{Text: "a"}}},
			{Number: 4, Verb: "Combine", Items: []Item{{Ref: 3}}},
		},
	}
	tbl := parseTable(t, string(RenderTable(r)))
	checkGridShape(t, tbl)
	ingredients, actions := ingredientRows(tbl)
	if len(ingredients) != 1 || len(actions) != 2 {
		t.Fatalf("got %d ingredient rows and %d action rows, want 1 and 2", len(ingredients), len(actions))
	}
	if got, want := actions[0].cells[0].text, "[1] Preheat"; got != want {
		t.Errorf("step with no items = %q, want %q", got, want)
	}
	if got, want := actions[1].cells[0].text, "[2] Wait [1] Preheat #9"; got != want {
		t.Errorf("action refs = %q, want %q", got, want)
	}
	if cell, ok := tbl.at(ingredients[0].index, 3); !ok || cell.text != "Combine" {
		t.Errorf("step 4 referencing step 3 should be a step cell in column 3, got %+v", cell)
	}

	// Duplicated numbers: #1 resolves to the first step numbered 1.
	dup := &Recipe{Steps: []Step{
		{Number: 1, Verb: "A"},
		{Number: 1, Verb: "B", Items: []Item{{Ref: 1}}},
		{Number: 2, Verb: "C", Items: []Item{{Text: "x"}}},
	}}
	dupTbl := parseTable(t, string(RenderTable(dup)))
	dupIngredients, dupActions := ingredientRows(dupTbl)
	if len(dupActions) != 2 {
		t.Fatalf("got %d action rows, want 2", len(dupActions))
	}
	if got, want := dupActions[1].cells[0].text, "[1] B [1] A"; got != want {
		t.Errorf("duplicate-number ref = %q, want %q (first appearance)", got, want)
	}
	if cell, ok := dupTbl.at(dupIngredients[0].index, 2); !ok || cell.text != "C" {
		t.Errorf("duplicate-number matrix row = %+v, want step C's cell", cell)
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
	cell, ok := strandedTbl.at(strandedIngredients[0].index, 3)
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
	matches, err := filepath.Glob(filepath.Join(inputDir, "*.trn.md"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no fixtures found under examples/input/")
	}
	names := make([]string, 0, len(matches))
	for _, path := range matches {
		names = append(names, filepath.Base(path))
	}
	return names
}
