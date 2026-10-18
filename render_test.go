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
// model: SPEC-ENGINE E-A2 asks for counts taken from the output, so every
// assertion below goes through parseTable.

var (
	tableRowPattern     = regexp.MustCompile(`(?s)<tr\b([^>]*)>(.*?)</tr>`)
	tableCellPattern    = regexp.MustCompile(`(?s)<(th|td)\b([^>]*)>(.*?)</(?:th|td)>`)
	tableAttrPattern    = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
	tableMarkupPattern  = regexp.MustCompile(`<[^>]*>`)
	tableCaptionPattern = regexp.MustCompile(`(?s)<caption\b[^>]*>(.*?)</caption>`)
)

type parsedCell struct {
	tag     string // "th" or "td"
	class   string
	colspan int
	text    string // rendered text, tags stripped and entities decoded
}

type parsedRow struct {
	class string
	cells []parsedCell
}

type parsedTable struct {
	caption string
	rows    []parsedRow
}

// parseTable reads the fragment back into rows and cells. It is deliberately
// small: the engine emits a fixed, well-formed subset of HTML, so a tag scan is
// enough to verify structure and cell contents.
func parseTable(t *testing.T, fragment string) parsedTable {
	t.Helper()
	var tbl parsedTable
	if m := tableCaptionPattern.FindStringSubmatch(fragment); m != nil {
		tbl.caption = textOf(m[1])
	}
	for _, rowMatch := range tableRowPattern.FindAllStringSubmatch(fragment, -1) {
		row := parsedRow{class: attrOf(rowMatch[1], "class")}
		for _, cellMatch := range tableCellPattern.FindAllStringSubmatch(rowMatch[2], -1) {
			cell := parsedCell{
				tag:   cellMatch[1],
				class: attrOf(cellMatch[2], "class"),
				text:  textOf(cellMatch[3]),
			}
			if n, err := strconv.Atoi(attrOf(cellMatch[2], "colspan")); err == nil {
				cell.colspan = n
			}
			row.cells = append(row.cells, cell)
		}
		tbl.rows = append(tbl.rows, row)
	}
	return tbl
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

// cellKind maps a cell class to the model's kind letter: d = directly listed,
// f = flow marker, e = empty.
func cellKind(c parsedCell) string {
	switch {
	case strings.Contains(c.class, "trn-cell-direct"):
		return "d"
	case strings.Contains(c.class, "trn-cell-flow"):
		return "f"
	case strings.Contains(c.class, "trn-cell-empty"):
		return "e"
	default:
		return "?"
	}
}

// kindsOf flattens one row's cell kinds into a compact string such as "dffff".
func kindsOf(t *testing.T, row parsedRow) string {
	t.Helper()
	var b strings.Builder
	for _, c := range row.cells {
		b.WriteString(cellKind(c))
	}
	return b.String()
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

// ingredientRows returns the matrix' ingredient rows (the ones carrying a
// scope=row header) and the action rows separately.
func ingredientRows(tbl parsedTable) (ingredients, actions []parsedRow) {
	for _, row := range tbl.rows {
		if strings.Contains(row.class, "trn-row") {
			ingredients = append(ingredients, row)
			continue
		}
		if strings.Contains(row.class, "trn-action") {
			actions = append(actions, row)
		}
	}
	return ingredients, actions
}

// TestEA2ChaiMatrix is acceptance criterion E-A2 against the rendered HTML:
// column count, row count, the action rows, the flow markers and the
// convergence of every ingredient into the final column.
//
// SPEC-ENGINE §7 phrases E-A2 as "11 ingredient rows × 5 step columns". The 11
// does not follow from §4.1 ("rows: unique ingredients, ordered by first
// appearance"): the chai document lists 8 unique ingredients. 11 is the number
// of items in steps 1–4 with the references counted as items (4+3+3+1); step 5
// adds one more ref for 12. §4.1 is the normative rendering rule, so the matrix
// has 8 ingredient rows; the discrepancy is recorded in the README and on the
// board rather than papered over by inventing three rows.
func TestEA2ChaiMatrix(t *testing.T) {
	r := loadRecipe(t, "chai.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	ingredients, actions := ingredientRows(tbl)

	if tbl.caption != "Masala Chai" {
		t.Errorf("caption = %q, want %q", tbl.caption, "Masala Chai")
	}

	// Columns: one header cell per step plus the ingredient corner, verbatim
	// `[n]` + verb headers.
	if len(tbl.rows) == 0 {
		t.Fatal("no table header row")
	}
	header := tbl.rows[0]
	if len(header.cells) != 6 {
		t.Fatalf("header has %d cells, want 6 (ingredient column + 5 steps)", len(header.cells))
	}
	if header.cells[0].tag != "th" || header.cells[0].text != "Ingredient" {
		t.Errorf("corner header = %q %q, want th %q", header.cells[0].tag, header.cells[0].text, "Ingredient")
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
		if cell.tag != "th" || cell.colspan != 0 {
			t.Errorf("step header %d: tag=%q colspan=%d, want a plain th", i+1, cell.tag, cell.colspan)
		}
		if cell.text != want {
			t.Errorf("step header %d = %q, want %q", i+1, cell.text, want)
		}
	}

	// Rows: unique ingredients in first-appearance order; d/f/e as above.
	wantRows := []struct {
		ingredient string
		kinds      string
	}{
		{"4 green cardamoms", "dffff"},
		{"2 cloves", "dffff"},
		{"3g cinnamon", "dffff"},
		{"2 peppercorns", "dffff"},
		{"1/2 inch ginger crushed", "edfff"},
		{"6g black tea", "edfff"},
		{"240ml full-fat milk", "eedff"},
		{"12g sugar", "eedff"},
	}
	if len(ingredients) != len(wantRows) {
		t.Fatalf("got %d ingredient rows, want %d", len(ingredients), len(wantRows))
	}
	markers := 0
	for i, want := range wantRows {
		row := ingredients[i]
		if len(row.cells) != 6 {
			t.Fatalf("row %d has %d cells, want 6 (row header + 5 steps)", i, len(row.cells))
		}
		if row.cells[0].tag != "th" || row.cells[0].text != want.ingredient {
			t.Errorf("row %d label = %q, want %q", i, row.cells[0].text, want.ingredient)
		}
		if got := kindsOf(t, parsedRow{cells: row.cells[1:]}); got != want.kinds {
			t.Errorf("row %q cells = %s, want %s", want.ingredient, got, want.kinds)
		}
		for _, c := range row.cells[1:] {
			if cellKind(c) == "f" {
				markers++
				if c.text != "" {
					t.Errorf("flow marker cell carries text %q; the marker must be a styled glyph only", c.text)
				}
			}
		}
		// Direct cells show the step's verb.
		for col, c := range row.cells[1:] {
			if cellKind(c) != "d" {
				continue
			}
			if wantVerb := r.Steps[col].Verb; c.text != wantVerb {
				t.Errorf("row %q column %d verb = %q, want %q", want.ingredient, col+1, c.text, wantVerb)
			}
		}
		// Convergence: every ingredient in this recipe reaches the final step,
		// so no raw in the last column may be empty.
		if last := row.cells[len(row.cells)-1]; cellKind(last) == "e" {
			t.Errorf("row %q has an empty final-column cell; every chai ingredient converges on [5]", want.ingredient)
		}
	}
	// 4 ingredients from step 1 carried through 4 more steps (16), 2 from step 2
	// through 3 (6), 2 from step 3 through 2 (4).
	if want := 26; markers != want {
		t.Errorf("flow markers = %d, want %d", markers, want)
	}

	// Steps 4 and 5 add no ingredient of their own: each becomes one spanning
	// action row across every column.
	if len(actions) != 2 {
		t.Fatalf("got %d action rows, want 2 (steps 4 and 5)", len(actions))
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
		row := actions[i]
		if len(row.cells) != 1 {
			t.Fatalf("action row %d has %d cells, want exactly 1", i, len(row.cells))
		}
		cell := row.cells[0]
		if cell.colspan != 6 {
			t.Errorf("action row %d colspan = %d, want 6 (all columns)", i, cell.colspan)
		}
		wantText := "[" + strconv.Itoa(want.number) + "] " + want.verb + " " + want.ref
		if cell.text != wantText {
			t.Errorf("action row %d text = %q, want %q", i, cell.text, wantText)
		}
	}
}

// TestGuacamoleMatrix pins the continuation-line example: refs and ingredients
// interleaved into one step, no action rows, markers only where a step feeds a
// later one.
func TestGuacamoleMatrix(t *testing.T) {
	r := loadRecipe(t, "guacamole.trn.md")
	tbl := parseTable(t, string(RenderTable(r)))
	ingredients, actions := ingredientRows(tbl)

	if len(actions) != 0 {
		t.Errorf("got %d action rows, want none: step 4 adds cilantro and salt", len(actions))
	}
	want := []struct {
		ingredient string
		kinds      string
	}{
		{"3 ripe avocados", "deef"},
		{"1/2 red onion", "edef"},
		{"1 jalapeno", "edef"},
		{"1 roma tomato", "edef"},
		{"1 lime", "eedf"},
		{"1/4 cup chopped cilantro", "eeed"},
		{"1 tsp salt", "eeed"},
	}
	if len(ingredients) != len(want) {
		t.Fatalf("got %d ingredient rows, want %d", len(ingredients), len(want))
	}
	for i, w := range want {
		row := ingredients[i]
		if row.cells[0].text != w.ingredient {
			t.Errorf("row %d label = %q, want %q", i, row.cells[0].text, w.ingredient)
		}
		if got := kindsOf(t, parsedRow{cells: row.cells[1:]}); got != w.kinds {
			t.Errorf("row %q cells = %s, want %s", w.ingredient, got, w.kinds)
		}
	}
}

// TestMatrixMatchesReachability re-derives the expected cell kinds from the AST
// with an independent walk for every fixture, and checks that the matrix agrees
// with the W4 channel: a row's final cell is empty exactly when its ingredient
// is reported as never reaching the final step.
func TestMatrixMatchesReachability(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			r := loadRecipe(t, name)
			tbl := parseTable(t, string(RenderTable(r)))
			ingredients, _ := ingredientRows(tbl)

			firstAt := map[int]int{}
			for i, step := range r.Steps {
				if _, seen := firstAt[step.Number]; !seen {
					firstAt[step.Number] = i
				}
			}
			reach := make([]map[int]bool, len(r.Steps))
			for col := range r.Steps {
				reach[col] = testReach(r.Steps, firstAt, col)
			}

			// Distinct ingredients in first-appearance order.
			var order []string
			seen := map[string]bool{}
			for _, step := range r.Steps {
				for _, item := range step.Items {
					if item.Ref == 0 && item.Text != "" && !seen[item.Text] {
						seen[item.Text] = true
						order = append(order, item.Text)
					}
				}
			}
			if len(ingredients) != len(order) {
				t.Fatalf("got %d ingredient rows, want one per unique ingredient (%d)", len(ingredients), len(order))
			}
			for i, text := range order {
				if got := ingredients[i].cells[0].text; got != text {
					t.Errorf("row %d label = %q, want %q (first appearance order)", i, got, text)
				}
				for col := range r.Steps {
					want := "e"
					if lists(r.Steps[col], text) {
						want = "d"
					} else {
						for i := range reach[col] {
							if i < len(r.Steps) && reach[col][i] && lists(r.Steps[i], text) {
								want = "f"
								break
							}
						}
					}
					if got := cellKind(ingredients[i].cells[col+1]); got != want {
						t.Errorf("cell (%q, column %d) = %s, want %s", text, col+1, got, want)
					}
				}
			}

			// Agreement with the W4 channel.
			stranded := testStranded(r)
			warnings := ConvergenceWarnings(r)
			if len(warnings) != len(stranded) {
				t.Fatalf("ConvergenceWarnings reported %d ingredients, matrix-derived count is %d", len(warnings), len(stranded))
			}
			for i, text := range stranded {
				if warnings[i].Code != "W4" || !strings.Contains(warnings[i].Msg, strconv.Quote(text)) {
					t.Errorf("warning %d = %+v, want W4 naming %q", i, warnings[i], text)
				}
				// The stranded ingredient's final-column cell must be empty.
				for _, row := range ingredients {
					if row.cells[0].text == text {
						if last := row.cells[len(row.cells)-1]; cellKind(last) != "e" {
							t.Errorf("stranded ingredient %q has non-empty final cell %s", text, cellKind(last))
						}
					}
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
// first-appearance order — the expectation ConvergenceWarnings must match.
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
			{Number: 3, Verb: verb2, Items: []Item{{Ref: 2}}},
		},
	}
	out := string(RenderTable(r))
	for _, bad := range []string{"<script", "<b>", "<i>", "<img", "<svg"} {
		if strings.Contains(out, bad) {
			t.Errorf("rendered HTML contains live markup %q from user text", bad)
		}
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the title's <script> to appear HTML-escaped")
	}

	tbl := parseTable(t, out)
	ingredients, actions := ingredientRows(tbl)
	if tbl.caption != title {
		t.Errorf("caption round-trip = %q, want %q", tbl.caption, title)
	}
	if len(ingredients) != 2 {
		t.Fatalf("got %d ingredient rows, want 2", len(ingredients))
	}
	for i, want := range []string{ing1, ing2} {
		if got := ingredients[i].cells[0].text; got != want {
			t.Errorf("ingredient %d round-trip = %q, want %q", i, got, want)
		}
	}
	if got := ingredients[0].cells[1].text; got != verb1 {
		t.Errorf("direct cell verb round-trip = %q, want %q", got, verb1)
	}
	if got := tbl.rows[0].cells[1].text; got != "[1] "+verb1 {
		t.Errorf("step header round-trip = %q, want %q", got, "[1] "+verb1)
	}
	if got := tbl.rows[0].cells[2].text; got != "[2] Fold <i>in</i>" {
		t.Errorf("step header round-trip = %q, want %q", got, "[2] Fold <i>in</i>")
	}
	if len(actions) != 1 {
		t.Fatalf("got %d action rows, want 1", len(actions))
	}
	wantAction := "[3] " + verb2 + " [2] Fold <i>in</i>"
	if got := actions[0].cells[0].text; got != wantAction {
		t.Errorf("action row round-trip = %q, want %q", got, wantAction)
	}
}

// TestRenderTableNoScriptNoExternal is the static half of "zero JavaScript, no
// external resources": nothing executable, nothing fetched, no images.
func TestRenderTableNoScriptNoExternal(t *testing.T) {
	for _, name := range fixtureNames(t) {
		out := string(RenderTable(loadRecipe(t, name)))
		for _, bad := range []string{"<script", "<img", "<svg", "<canvas", "<link", "src=", "javascript:", "@import", "url(", "http://", "https://"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s: rendered HTML contains %q", name, bad)
			}
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
		})
	}
}

// TestRenderTableDegenerateSteps pins the less obvious structural choices:
// a step with no items at all is an action row, a dangling reference renders by
// number, and a duplicated step number resolves to its first appearance.
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
	if got, want := kindsOf(t, parsedRow{cells: dupIngredients[0].cells[1:]}), "df"; got != want {
		t.Errorf("duplicate-number matrix row = %s, want %s", got, want)
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
