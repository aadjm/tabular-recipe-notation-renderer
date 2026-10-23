package trn

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func recipe(title string, steps ...Step) *Recipe {
	return &Recipe{Title: title, Steps: steps}
}

func step(number int, verb string, items ...Item) Step {
	return Step{Number: number, Verb: verb, Items: items}
}

func ing(text string) Item { return Item{Text: text} }

func ref(n int) Item { return Item{Ref: n} }

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(inputDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

var validCodes = map[string]bool{
	"W1": true, "W2": true, "W3": true,
	"W4": true, "W5": true, "W6": true, "W7": true,
}

func codes(warnings []Warning) []string {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]string, len(warnings))
	for i, w := range warnings {
		out[i] = w.Code
	}
	return out
}

// check parses input, compares the AST with want and asserts that every
// warning carries a known code and a non-empty message. It returns the recipe
// and the parse-time warnings; W4 is render-time and comes from
// ConvergenceWarnings (see warnings_test.go).
func check(t *testing.T, input string, want *Recipe) (*Recipe, []Warning) {
	t.Helper()
	got, warnings := Parse(input)
	if got == nil {
		t.Fatalf("Parse returned a nil recipe")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recipe mismatch\n got: %s\nwant: %s", dump(got), dump(want))
	}
	for i, w := range warnings {
		if !validCodes[w.Code] {
			t.Errorf("warning %d has unknown code %q", i, w.Code)
		}
		if strings.TrimSpace(w.Msg) == "" {
			t.Errorf("warning %d (%s) has an empty message", i, w.Code)
		}
	}
	return got, warnings
}

// dump renders a recipe compactly so failures are readable.
func dump(r *Recipe) string {
	var b strings.Builder
	b.WriteString("{title: " + quote(r.Title) + " steps: [")
	for i, s := range r.Steps {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString("[" + strconv.Itoa(s.Number) + "] " + quote(s.Verb) + " {")
		for j, item := range s.Items {
			if j > 0 {
				b.WriteString(", ")
			}
			if item.Ref > 0 {
				b.WriteString("#" + strconv.Itoa(item.Ref))
			} else {
				b.WriteString("ing " + quote(item.Text))
			}
		}
		b.WriteString("}}")
	}
	b.WriteString("]}")
	return b.String()
}

func quote(s string) string { return "\"" + s + "\"" }

// ---- canonical fixtures (SPEC-ENGINE §2, §6) -------------------------------

func TestParseCanonicalFixtures(t *testing.T) {
	cases := []struct {
		name string
		file string
		want *Recipe
	}{
		{
			name: "masala chai (single-line steps)",
			file: "chai.trn.md",
			want: recipe("Masala Chai",
				step(1, "Crush or grind to a powder",
					ing("4 green cardamoms"), ing("2 cloves"), ing("3g cinnamon"), ing("2 peppercorns")),
				step(2, "Boil 3 min in 360ml water until the decoction is dark",
					ref(1), ing("1/2 inch ginger crushed"), ing("6g black tea")),
				step(3, "Add and bring to a boil",
					ref(2), ing("240ml full-fat milk"), ing("12g sugar")),
				step(4, "Simmer 2 min until dark with a cream layer and no raw milk flavour",
					ref(3)),
				step(5, "Pull 4-5 times from 1-1.5 ft, then strain into cups and serve",
					ref(4)),
			),
		},
		{
			name: "guacamole (continuation lines)",
			file: "guacamole.trn.md",
			want: recipe("Guacamole",
				step(1, "Mash", ing("3 ripe avocados")),
				step(2, "Finely dice", ing("1/2 red onion"), ing("1 jalapeno"), ing("1 roma tomato")),
				step(3, "Juice", ing("1 lime")),
				step(4, "Fold together", ref(1), ref(2), ref(3),
					ing("1/4 cup chopped cilantro"), ing("1 tsp salt")),
			),
		},
		{
			// Verbatim from TheSonOfThomp/recipe-parser README: indented steps,
			// trailing spaces after "Stir: " and after each continuation comma.
			name: "kraft dinner (recipe-parser README, verbatim)",
			file: "kraft-dinner.trn.md",
			want: recipe("Kraft Dinner",
				step(1, "Boil", ing("6 cups water")),
				step(2, "Cook for 6 minutes", ref(1), ing("macaroni")),
				step(3, "Drain", ref(2)),
				step(4, "Stir", ref(3), ing("1/4 cup butter"), ing("1/4 cup milk"), ing("powdered cheese")),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := readFixture(t, tc.file)
			got, warnings := check(t, input, tc.want)
			// The canonical examples must be clean on the full channel: parse
			// warnings plus the render-time convergence rule (W4).
			all := append(warnings, ConvergenceWarnings(got)...)
			if len(all) != 0 {
				t.Errorf("canonical example produced warnings: %v", all)
			}
		})
	}
}

// E-A3: guacamole step 4's continuation lines parse into 5 items in order.
func TestContinuationLinesPreserveOrder(t *testing.T) {
	got, _ := Parse(readFixture(t, "guacamole.trn.md"))
	if len(got.Steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(got.Steps))
	}
	items := got.Steps[3].Items
	want := []Item{ref(1), ref(2), ref(3), ing("1/4 cup chopped cilantro"), ing("1 tsp salt")}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("step 4 items = %v, want %v", items, want)
	}
}

// TestParseLiveGrocyFixtures parses the five real recipes carried by the `trn`
// userfield on the homelab Grocy instance. The fixtures were fetched read-only
// and hold recipe text only — never the API key. They are the documents E-A1
// renders, so they must parse warning-free.
func TestParseLiveGrocyFixtures(t *testing.T) {
	wantSteps := map[int]int{1: 10, 2: 4, 3: 4, 4: 6, 5: 5}
	for id := 1; id <= 5; id++ {
		file := "grocy-" + strconv.Itoa(id) + ".trn.md"
		t.Run(file, func(t *testing.T) {
			got, warnings := Parse(readFixture(t, file))
			all := append(warnings, ConvergenceWarnings(got)...)
			if len(all) != 0 {
				t.Errorf("warnings: %v", all)
			}
			if got.Title == "" {
				t.Error("empty title")
			}
			if len(got.Steps) != wantSteps[id] {
				t.Fatalf("steps = %d, want %d", len(got.Steps), wantSteps[id])
			}
			numbers := make(map[int]bool, len(got.Steps))
			for _, s := range got.Steps {
				numbers[s.Number] = true
			}
			for _, s := range got.Steps {
				if s.Verb == "" {
					t.Errorf("step [%d] has an empty verb", s.Number)
				}
				if len(s.Items) == 0 {
					t.Errorf("step [%d] has no items", s.Number)
				}
				for _, item := range s.Items {
					if item.Ref > 0 && !numbers[item.Ref] {
						t.Errorf("step [%d] references missing #%d", s.Number, item.Ref)
					}
				}
			}
		})
	}
}

// ---- edge cases (SPEC-ENGINE §6 parser list) -------------------------------

func TestParseEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  *Recipe
		codes []string
	}{
		{
			name:  "empty input",
			input: "",
			want:  recipe(""),
			codes: []string{"W5", "W6"},
		},
		{
			name:  "whitespace only",
			input: "   \n\t\n\n  \t \n",
			want:  recipe(""),
			codes: []string{"W5", "W6"},
		},
		{
			name:  "document opens with a step: no title",
			input: "[1] Mix: flour\n[2] Bake: #1\n",
			want: recipe("",
				step(1, "Mix", ing("flour")),
				step(2, "Bake", ref(1)),
			),
			codes: []string{"W6"},
		},
		{
			name:  "no steps",
			input: "Just a title\n",
			want:  recipe("Just a title"),
			codes: []string{"W5"},
		},
		{
			name:  "step line without a colon",
			input: "Toast\n[1] Toast the bread\n",
			want: recipe("Toast",
				step(1, "Toast the bread"),
			),
		},
		{
			name:  "step line without a colon and no verb",
			input: "Toast\n[1]\n",
			want: recipe("Toast",
				step(1, ""),
			),
			codes: []string{"W7"},
		},
		{
			name:  "step line with an empty verb and items",
			input: "Toast\n[1] : bread\n",
			want: recipe("Toast",
				step(1, "", ing("bread")),
			),
			codes: []string{"W7"},
		},
		{
			name:  "blank lines between steps are ignored",
			input: "Toast\n\n[1] Toast the bread\n\n\n[2] Butter: #1, butter\n\n",
			want: recipe("Toast",
				step(1, "Toast the bread"),
				step(2, "Butter", ref(1), ing("butter")),
			),
		},
		{
			name:  "trailing whitespace and trailing newline",
			input: "Toast   \n[1] Butter: #1, butter   \n\n\n   ",
			want: recipe("Toast",
				step(1, "Butter", ref(1), ing("butter")),
			),
		},
		{
			name:  "duplicated step numbers are kept, refs resolve to the first",
			input: "Toast\n[1] A: x\n[2] B: #1\n[2] C: #2\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(2, "B", ref(1)),
				step(2, "C", ref(2)),
			),
			codes: []string{"W1"},
		},
		{
			name:  "gap in step numbers",
			input: "Toast\n[1] A: x\n[3] B: #1\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(3, "B", ref(1)),
			),
			codes: []string{"W1"},
		},
		{
			name:  "out of order step numbers",
			input: "Toast\n[3] A: x\n[2] B: #1\n[1] C: y\n",
			want: recipe("Toast",
				step(3, "A", ing("x")),
				step(2, "B", ref(1)),
				step(1, "C", ing("y")),
			),
			codes: []string{"W1", "W1", "W3"},
		},
		{
			name:  "leading zeroes are not a sequence problem",
			input: "Toast\n[01] A: x\n[02] B: #1\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(2, "B", ref(1)),
			),
		},
		{
			name:  "dangling reference",
			input: "Toast\n[1] A: x, #9\n",
			want: recipe("Toast",
				step(1, "A", ing("x"), ref(9)),
			),
			codes: []string{"W2"},
		},
		{
			name:  "forward reference",
			input: "Toast\n[1] A: #2\n[2] B: x\n",
			want: recipe("Toast",
				step(1, "A", ref(2)),
				step(2, "B", ing("x")),
			),
			codes: []string{"W3"},
		},
		{
			name:  "references in continuation lines",
			input: "Toast\n[1] A: x\n[2] B:\n  #1,\n  y\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(2, "B", ref(1), ing("y")),
			),
		},
		{
			name:  "items with extra spaces are trimmed, interior spacing kept",
			input: "Toast\n[1]   Mix:    1 cup   flour ,  sugar ,  #1  \n",
			want: recipe("Toast",
				step(1, "Mix", ing("1 cup   flour"), ing("sugar"), ref(1)),
			),
		},
		{
			name:  "fractions, parentheses and unicode preserved verbatim",
			input: "Toast\n[1] Mix: 2 1/2 cups (400 g) flour, ½ tsp ground ñora, 1–2 dashes\n",
			want: recipe("Toast",
				step(1, "Mix", ing("2 1/2 cups (400 g) flour"), ing("½ tsp ground ñora"), ing("1–2 dashes")),
			),
		},
		{
			name:  "CRLF line endings",
			input: "Toast\r\n[1] A: x\r\n[2] B: #1\r\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(2, "B", ref(1)),
			),
		},
		{
			name:  "UTF-8 BOM is not part of the title",
			input: "\ufeffToast\n[1] A: x\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
			),
		},
		{
			name:  "#0 cannot be a reference, so it stays ingredient text",
			input: "Toast\n[1] A: #0, x\n",
			want: recipe("Toast",
				step(1, "A", ing("#0"), ing("x")),
			),
		},
		{
			name:  "stray text before the first step is dropped",
			input: "Toast\nstray text\n[1] A: x\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
			),
		},
		{
			name:  "a continuation line may follow a colon-less step",
			input: "Toast\n[1] Mix\n  flour\n  milk\n",
			want: recipe("Toast",
				step(1, "Mix", ing("flour"), ing("milk")),
			),
		},
		{
			name:  "empty items are dropped, not reported",
			input: "Toast\n[1] A: , ,\n",
			want: recipe("Toast",
				step(1, "A"),
			),
		},
		{
			name:  "step number too large for an int degrades to W1",
			input: "Toast\n[99999999999999999999] A: x\n",
			want: recipe("Toast",
				step(0, "A", ing("x")),
			),
			codes: []string{"W1"},
		},
		{
			name:  "indented step lines are steps, not continuations",
			input: "Toast\n  [1] A: x\n  [2] B: #1\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
				step(2, "B", ref(1)),
			),
		},
		{
			name:  "indented title",
			input: "   Toast\n[1] A: x\n",
			want: recipe("Toast",
				step(1, "A", ing("x")),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := check(t, tc.input, tc.want)
			if got := codes(warnings); !reflect.DeepEqual(got, tc.codes) {
				t.Errorf("warning codes = %v, want %v", got, tc.codes)
			}
		})
	}
}

// ---- robustness (E-A4, partial) --------------------------------------------

// TestParseNeverPanics feeds pathological documents through the parser. Every
// input must yield a usable recipe with valid warnings — never a panic.
func TestParseNeverPanics(t *testing.T) {
	inputs := []string{
		"",
		"\x00\x00\x00",
		"[",
		"]",
		"[]",
		"[1]",
		"[1:",
		"[1] : ",
		"#",
		"#1",
		"#1: nothing",
		"Title\n" + strings.Repeat("  ,\n", 100),
		"Title\n[1] A: " + strings.Repeat(",", 100),
		"Title\n" + strings.Repeat("[1] A: x\n", 50),
		"Title\n[1] A: #999999999999999999999\n",
		"Title\n[0] A: x\n",
		"\ufeff",
		"\r\n\r\n",
		"  \n\t\n",
		strings.Repeat("\n", 1000),
		"[1] A: x\n" + strings.Repeat("\xff\xfe", 8),
	}
	for _, input := range inputs {
		got, _ := Parse(input)
		if got == nil {
			t.Fatalf("Parse(%q) returned nil", input)
		}
	}
}

// FuzzParse checks the parser's invariants on arbitrary input.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"Masala Chai\n[1] Crush: 4 cardamoms\n[2] Boil: #1, water\n",
		"  [4] Stir: \n    #3, \n    1/4 cup butter, \n    powdered cheese\n",
		"[]", "[1]", "[1] : x", "#0", "Title\n  stray\n[1] A\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		got, warnings := Parse(input)
		if got == nil {
			t.Fatal("nil recipe")
		}
		for _, s := range got.Steps {
			if s.Number < 0 {
				t.Fatalf("negative step number %d", s.Number)
			}
			for _, item := range s.Items {
				if item.Ref > 0 && item.Text != "" {
					t.Fatalf("item %#v is both a reference and text", item)
				}
				if item.Ref == 0 {
					if item.Text == "" {
						t.Fatalf("empty ingredient item in step %d", s.Number)
					}
					if strings.TrimSpace(item.Text) != item.Text {
						t.Fatalf("ingredient %q has surrounding whitespace", item.Text)
					}
				}
			}
		}
		for _, w := range warnings {
			if !validCodes[w.Code] || w.Msg == "" {
				t.Fatalf("bad warning %#v", w)
			}
		}
		// Warnings are always returned in code order.
		for i := 1; i < len(warnings); i++ {
			if warnings[i-1].Code > warnings[i].Code {
				t.Fatalf("warnings out of order: %v", codes(warnings))
			}
		}
		// The render-time half of the channel must hold the same invariants,
		// and it can never report more stranded ingredients than the document
		// has distinct ingredient strings.
		seen := make(map[string]bool, len(warnings))
		for _, s := range got.Steps {
			for _, item := range s.Items {
				if item.Ref == 0 {
					seen[item.Text] = true
				}
			}
		}
		renderWarnings := ConvergenceWarnings(got)
		if len(renderWarnings) > len(seen) {
			t.Fatalf("%d W4s for %d distinct ingredients", len(renderWarnings), len(seen))
		}
		for _, w := range renderWarnings {
			if w.Code != "W4" || w.Msg == "" {
				t.Fatalf("bad render warning %#v", w)
			}
		}

		// The matrix renderer takes the same arbitrary input: it must never
		// panic, must escape user text, and must keep one row per distinct
		// ingredient and one spanning row per ingredient-free step.
		fragment := string(RenderTable(got))
		if strings.Contains(fragment, "<script") {
			t.Fatalf("rendered matrix contains an unescaped script tag")
		}
		if gotRows := strings.Count(fragment, `<tr class="trn-row">`); gotRows != len(seen) {
			t.Fatalf("%d matrix rows for %d distinct ingredients", gotRows, len(seen))
		}
		wantActions := 0
		for _, s := range got.Steps {
			adds := false
			for _, item := range s.Items {
				if item.Ref == 0 {
					adds = true
				}
			}
			if !adds {
				wantActions++
			}
		}
		if gotActions := strings.Count(fragment, `<tr class="trn-action">`); gotActions != wantActions {
			t.Fatalf("%d action rows for %d ingredient-free steps", gotActions, wantActions)
		}
		if RenderTable(nil) != "" {
			t.Fatal("RenderTable(nil) is not the empty fragment")
		}

		// The flow renderer takes the same arbitrary input: one box per step,
		// one chip per *first* appearance of an ingredient string (SPEC-ENGINE
		// §4.2 — a later re-listing is not a second chip), and no unescaped
		// script tag.
		flow := string(RenderFlow(got))
		if strings.Contains(flow, "<script") {
			t.Fatalf("rendered flow view contains an unescaped script tag")
		}
		if gotBoxes := strings.Count(flow, `<li class="trn-flow-step`); gotBoxes != len(got.Steps) {
			t.Fatalf("%d flow boxes for %d steps", gotBoxes, len(got.Steps))
		}
		distinct := make(map[string]bool)
		for _, s := range got.Steps {
			for _, item := range s.Items {
				if item.Ref == 0 && item.Text != "" {
					distinct[item.Text] = true
				}
			}
		}
		chips := strings.Count(flow, `<li class="trn-chip">`)
		if chips != len(distinct) {
			t.Fatalf("%d chips for %d distinct ingredient texts", chips, len(distinct))
		}
		if RenderFlow(nil) != "" {
			t.Fatal("RenderFlow(nil) is not the empty fragment")
		}
	})
}
