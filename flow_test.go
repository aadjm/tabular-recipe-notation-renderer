package trn

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The tests in this file read the rendered flow fragment back out of the HTML
// rather than trusting the model, the same way render_test.go does for the
// matrix: structure, numbers, chips and connectors are all asserted on the
// bytes a browser would receive.

var (
	flowTitlePattern  = regexp.MustCompile(`(?s)<figcaption class="trn-title trn-flow-title">(.*?)</figcaption>`)
	flowBoxStart      = regexp.MustCompile(`<li class="trn-flow-step([^"]*)" id="trn-flow-step-(\d+)">`)
	flowNumPattern    = regexp.MustCompile(`(?s)<span class="trn-flow-num">(.*?)</span>`)
	flowVerbPattern   = regexp.MustCompile(`(?s)<span class="trn-flow-verb">(.*?)</span>`)
	flowChipListStart = `<ul class="trn-flow-chips">`
	flowChipPattern   = regexp.MustCompile(`(?s)<li class="trn-chip">(.*?)</li>`)
	flowArrowsPattern = regexp.MustCompile(`(?s)<p class="trn-flow-arrows">(.*?)</p>`)
	flowArrowSplit    = regexp.MustCompile(`<span class="trn-flow-arrow( trn-flow-arrow-unknown)?"`)
	flowArrowFrom     = regexp.MustCompile(`data-from="(\d+)"`)
	flowArrowTo       = regexp.MustCompile(`data-to="(\d+)"`)
	flowArrowNum      = regexp.MustCompile(`(?s)<span class="trn-flow-arrow-num">(.*?)</span>`)
	flowArrowVerb     = regexp.MustCompile(`(?s)<span class="trn-flow-arrow-verb">(.*?)</span>`)
)

// parsedFlowArrow is one connector read back out of the fragment.
type parsedFlowArrow struct {
	from    int // 0 when the reference names no existing step
	to      int
	label   string // "[n]" or "#n" as rendered
	verb    string // the referenced step's verb; empty when dangling
	unknown bool
}

type parsedFlowBox struct {
	class  string
	pos    int
	body   string // markup of the box, used for empty-element checks
	num    string // "[n]" as rendered
	verb   string
	chips  []string
	arrows []parsedFlowArrow
}

type parsedFlow struct {
	title string
	boxes []parsedFlowBox
}

// parseFlow reads a flow fragment back into boxes. The engine emits a fixed
// subset of HTML, so locating each box start and scanning its slice is enough:
// chips are <li>s inside the box and never match the box start, and the slice
// runs to the next box start.
func parseFlow(t *testing.T, fragment string) parsedFlow {
	t.Helper()
	var flow parsedFlow
	if m := flowTitlePattern.FindStringSubmatch(fragment); m != nil {
		flow.title = textOf(m[1])
	}
	starts := flowBoxStart.FindAllStringSubmatchIndex(fragment, -1)
	for i, loc := range starts {
		end := len(fragment)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		pos, err := strconv.Atoi(fragment[loc[4]:loc[5]])
		if err != nil {
			t.Fatalf("box position %q is not an integer", fragment[loc[4]:loc[5]])
		}
		box := parsedFlowBox{
			class: fragment[loc[2]:loc[3]],
			pos:   pos,
			body:  fragment[loc[1]:end],
		}
		if m := flowNumPattern.FindStringSubmatch(box.body); m != nil {
			box.num = textOf(m[1])
		}
		if m := flowVerbPattern.FindStringSubmatch(box.body); m != nil {
			box.verb = textOf(m[1])
		}
		for _, m := range flowChipPattern.FindAllStringSubmatch(box.body, -1) {
			box.chips = append(box.chips, textOf(m[1]))
		}
		if m := flowArrowsPattern.FindStringSubmatch(box.body); m != nil {
			box.arrows = parseFlowArrows(t, m[1])
		}
		flow.boxes = append(flow.boxes, box)
	}
	return flow
}

// parseFlowArrows splits the arrows paragraph at every connector start, so each
// chunk holds one connector's ends, label and (for a resolved arrow) its verb.
func parseFlowArrows(t *testing.T, inner string) []parsedFlowArrow {
	t.Helper()
	var arrows []parsedFlowArrow
	starts := flowArrowSplit.FindAllStringSubmatchIndex(inner, -1)
	for i, loc := range starts {
		end := len(inner)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		chunk := inner[loc[1]:end]
		arrow := parsedFlowArrow{unknown: loc[2] >= 0}
		if m := flowArrowFrom.FindStringSubmatch(chunk); m != nil {
			arrow.from = positiveInt(t, m[1])
		}
		if m := flowArrowTo.FindStringSubmatch(chunk); m != nil {
			arrow.to = positiveInt(t, m[1])
		} else {
			t.Fatalf("connector without data-to: %q", chunk)
		}
		if m := flowArrowNum.FindStringSubmatch(chunk); m != nil {
			arrow.label = textOf(m[1])
		}
		if m := flowArrowVerb.FindStringSubmatch(chunk); m != nil {
			arrow.verb = textOf(m[1])
		}
		arrows = append(arrows, arrow)
	}
	return arrows
}

// positiveInt parses a rendered positive integer attribute value.
func positiveInt(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		t.Fatalf("%q is not a positive integer", s)
	}
	return n
}

// arrowLabels renders a box's connectors the way the fragment does, for
// comparisons against expectations.
func arrowLabels(box parsedFlowBox) []string {
	labels := make([]string, 0, len(box.arrows))
	for _, arrow := range box.arrows {
		labels = append(labels, arrow.label)
	}
	return labels
}

// arrowSources lists a box's connector sources in written order.
func arrowSources(box parsedFlowBox) []int {
	sources := make([]int, 0, len(box.arrows))
	for _, arrow := range box.arrows {
		sources = append(sources, arrow.from)
	}
	return sources
}

// TestFlowChaiBoxes checks the canonical single-line-steps example box by box:
// numbers, verbs, the chips each step introduces, and the connector each step
// draws from the one before it — including the box position it points at.
func TestFlowChaiBoxes(t *testing.T) {
	r := loadRecipe(t, "chai.trn.md")
	flow := parseFlow(t, string(RenderFlow(r)))
	if flow.title != "" {
		t.Errorf("default title = %q, want empty (hidden by default)", flow.title)
	}
	titledFlow := parseFlow(t, string(RenderFlowWithTitle(r, true)))
	if titledFlow.title != "Masala Chai" {
		t.Errorf("titled flow title = %q, want %q", titledFlow.title, "Masala Chai")
	}
	want := []struct {
		num, verb string
		chips     []string
		from      int // 0 = no connector
		label     string
	}{
		{"[1]", "Crush or grind to a powder",
			[]string{"4 green cardamoms", "2 cloves", "3g cinnamon", "2 peppercorns"}, 0, ""},
		{"[2]", "Boil 3 min in 360ml water until the decoction is dark",
			[]string{"1/2 inch ginger crushed", "6g black tea"}, 1, "[1]"},
		{"[3]", "Add and bring to a boil",
			[]string{"240ml full-fat milk", "12g sugar"}, 2, "[2]"},
		{"[4]", "Simmer 2 min until dark with a cream layer and no raw milk flavour",
			nil, 3, "[3]"},
		{"[5]", "Pull 4-5 times from 1-1.5 ft, then strain into cups and serve",
			nil, 4, "[4]"},
	}
	if len(flow.boxes) != len(want) {
		t.Fatalf("got %d boxes, want %d", len(flow.boxes), len(want))
	}
	for i, exp := range want {
		box := flow.boxes[i]
		if box.num != exp.num || box.verb != exp.verb {
			t.Errorf("box %d = %s %q, want %s %q", i, box.num, box.verb, exp.num, exp.verb)
		}
		if box.pos != i+1 {
			t.Errorf("box %s position = %d, want %d", box.num, box.pos, i+1)
		}
		if strings.Join(box.chips, "|") != strings.Join(exp.chips, "|") {
			t.Errorf("box %s chips = %q, want %q", box.num, box.chips, exp.chips)
		}
		if exp.from == 0 {
			if len(box.arrows) != 0 {
				t.Errorf("box %s has connectors %q, want none", box.num, arrowLabels(box))
			}
			continue
		}
		if len(box.arrows) != 1 {
			t.Errorf("box %s connectors = %q, want exactly one", box.num, arrowLabels(box))
			continue
		}
		arrow := box.arrows[0]
		if arrow.from != exp.from || arrow.to != i+1 || arrow.label != exp.label {
			t.Errorf("box %s connector = from %d to %d %q, want from %d to %d %q",
				box.num, arrow.from, arrow.to, arrow.label, exp.from, i+1, exp.label)
		}
		if arrow.verb != want[exp.from-1].verb {
			t.Errorf("box %s connector verb = %q, want %q", box.num, arrow.verb, want[exp.from-1].verb)
		}
		if arrow.unknown {
			t.Errorf("box %s connector is classed unknown", box.num)
		}
	}
}

// TestFlowGuacamoleContinuation covers the example whose step 4 continues over
// several lines: the continuation items must attach to that box — one connector
// per reference and the two ingredients as chips, in written order.
func TestFlowGuacamoleContinuation(t *testing.T) {
	flow := parseFlow(t, string(RenderFlow(loadRecipe(t, "guacamole.trn.md"))))
	if len(flow.boxes) != 4 {
		t.Fatalf("got %d boxes, want 4", len(flow.boxes))
	}
	wantChips := [][]string{
		{"3 ripe avocados"},
		{"1/2 red onion", "1 jalapeno", "1 roma tomato"},
		{"1 lime"},
		{"1/4 cup chopped cilantro", "1 tsp salt"},
	}
	wantLabels := [][]string{nil, nil, nil, {"[1]", "[2]", "[3]"}}
	for i := range flow.boxes {
		if got := flow.boxes[i].chips; strings.Join(got, "|") != strings.Join(wantChips[i], "|") {
			t.Errorf("box %d chips = %q, want %q", i+1, got, wantChips[i])
		}
		if got := arrowLabels(flow.boxes[i]); strings.Join(got, "|") != strings.Join(wantLabels[i], "|") {
			t.Errorf("box %d connectors = %q, want %q", i+1, got, wantLabels[i])
		}
	}
	box := flow.boxes[3]
	if got, want := arrowSources(box), []int{1, 2, 3}; !equalInts(got, want) {
		t.Errorf("box [4] connector sources = %v, want %v", got, want)
	}
	for _, arrow := range box.arrows {
		if arrow.to != 4 {
			t.Errorf("box [4] connector targets %d, want 4", arrow.to)
		}
	}
	if got := box.arrows[0].verb; got != "Mash" {
		t.Errorf("box [4] first connector verb = %q, want %q", got, "Mash")
	}
}

// TestFlowStructureMatchesAST compares every box of every fixture with the AST:
// box count, numbers, verbs, chips, and one connector per written reference, in
// order, resolved through the first appearance of the step number.
func TestFlowStructureMatchesAST(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		flow := parseFlow(t, string(RenderFlow(r)))
		if len(flow.boxes) != len(r.Steps) {
			t.Errorf("%s: %d boxes for %d steps", name, len(flow.boxes), len(r.Steps))
			continue
		}
		firstAt := firstByNumber(r)
		seen := make(map[string]bool)
		for i, step := range r.Steps {
			box := flow.boxes[i]
			if box.num != "["+strconv.Itoa(step.Number)+"]" || box.verb != step.Verb {
				t.Errorf("%s box %d = %s %q, want [%d] %q", name, i, box.num, box.verb, step.Number, step.Verb)
			}
			var wantChips []string
			var wantFrom []int
			var wantLabels, wantVerbs []string
			for _, item := range step.Items {
				if item.Ref <= 0 {
					if item.Text != "" && !seen[item.Text] {
						seen[item.Text] = true
						wantChips = append(wantChips, item.Text)
					}
					continue
				}
				j, ok := firstAt[item.Ref]
				if !ok {
					wantFrom = append(wantFrom, 0)
					wantLabels = append(wantLabels, "#"+strconv.Itoa(item.Ref))
					wantVerbs = append(wantVerbs, "")
					continue
				}
				wantFrom = append(wantFrom, j+1)
				wantLabels = append(wantLabels, "["+strconv.Itoa(item.Ref)+"]")
				wantVerbs = append(wantVerbs, r.Steps[j].Verb)
			}
			if strings.Join(box.chips, "|") != strings.Join(wantChips, "|") {
				t.Errorf("%s box %d chips = %q, want %q", name, i, box.chips, wantChips)
			}
			if got := arrowSources(box); !equalInts(got, wantFrom) {
				t.Errorf("%s box %d connector sources = %v, want %v", name, i, got, wantFrom)
			}
			if got := arrowLabels(box); strings.Join(got, "|") != strings.Join(wantLabels, "|") {
				t.Errorf("%s box %d connector labels = %q, want %q", name, i, got, wantLabels)
			}
			gotVerbs := make([]string, 0, len(box.arrows))
			for _, arrow := range box.arrows {
				gotVerbs = append(gotVerbs, arrow.verb)
			}
			if strings.Join(gotVerbs, "|") != strings.Join(wantVerbs, "|") {
				t.Errorf("%s box %d connector verbs = %q, want %q", name, i, gotVerbs, wantVerbs)
			}
			for _, arrow := range box.arrows {
				if arrow.to != i+1 {
					t.Errorf("%s box %d connector targets %d, want %d", name, i, arrow.to, i+1)
				}
			}
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFlowChipsFirstAppearanceOnly checks the chip rule against every fixture:
// each box carries exactly the ingredient strings that first appear in its step,
// in written order, and every distinct ingredient text of the document is a chip
// exactly once (SPEC-ENGINE §4.2).
func TestFlowChipsFirstAppearanceOnly(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		flow := parseFlow(t, string(RenderFlow(r)))
		seen := make(map[string]bool)
		for i, step := range r.Steps {
			if i >= len(flow.boxes) {
				break
			}
			var want []string
			for _, item := range step.Items {
				if item.Ref == 0 && item.Text != "" && !seen[item.Text] {
					seen[item.Text] = true
					want = append(want, item.Text)
				}
			}
			if got := flow.boxes[i].chips; strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("%s box %d chips = %q, want %q", name, i, got, want)
			}
			within := make(map[string]bool)
			for _, chip := range flow.boxes[i].chips {
				if within[chip] {
					t.Errorf("%s box %d carries chip %q twice", name, i, chip)
				}
				within[chip] = true
			}
		}
		distinct := make(map[string]bool)
		for _, step := range r.Steps {
			for _, item := range step.Items {
				if item.Ref == 0 && item.Text != "" {
					distinct[item.Text] = true
				}
			}
		}
		if len(seen) != len(distinct) {
			t.Errorf("%s: %d chips for %d distinct ingredients", name, len(seen), len(distinct))
		}
	}
}

// TestFlowRepeatedIngredient covers chicken-curry, where "1 tbsp cilantro" is listed
// twice: it is a chip on step 7 — where it first appears — and step 10, which
// lists nothing new, carries no chip for it (SPEC-ENGINE §4.2 attaches chips to
// the first appearance only).
func TestFlowRepeatedIngredient(t *testing.T) {
	fragment := string(RenderFlow(loadRecipe(t, "chicken-curry.trn.md")))
	flow := parseFlow(t, fragment)
	if len(flow.boxes) != 10 {
		t.Fatalf("got %d boxes, want 10", len(flow.boxes))
	}
	if !containsString(flow.boxes[6].chips, "1 tbsp cilantro") {
		t.Errorf("step 7 chips = %q, want to introduce %q", flow.boxes[6].chips, "1 tbsp cilantro")
	}
	if got := flow.boxes[9].chips; len(got) != 0 {
		t.Errorf("step 10 chips = %q, want none (its only ingredient was introduced in step 7)", got)
	}
	if got := flow.boxes[8].chips; len(got) != 1 || got[0] != "1/2 cup hot water" {
		t.Errorf("step 9 chips = %q, want [%q]", got, "1/2 cup hot water")
	}
	if !strings.Contains(fragment, `<ul class="trn-flow-chips">`) {
		t.Error("no chip list at all")
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestFlowFinalStepClass checks that exactly one box — the last step in document
// order — carries the trn-flow-final class, for every fixture, and that nothing
// else in the fragment claims to be final.
func TestFlowFinalStepClass(t *testing.T) {
	for _, name := range fixtureNames(t) {
		fragment := string(RenderFlow(loadRecipe(t, name)))
		flow := parseFlow(t, fragment)
		for i, box := range flow.boxes {
			final := strings.Contains(" "+box.class+" ", " trn-flow-final ")
			if final != (i == len(flow.boxes)-1) {
				t.Errorf("%s: box %d final = %v, want %v", name, i+1, final, i == len(flow.boxes)-1)
			}
		}
		if got := strings.Count(fragment, "trn-flow-final"); got != 1 {
			t.Errorf("%s: %d trn-flow-final occurrences, want 1", name, got)
		}
	}
}

// TestFlowConnectorsPointAtRenderedBoxes is the structural promise of §4.2's
// arrows: every connector points at the box that writes the reference, and every
// resolved connector's source is a box in the same fragment — by the same
// 1-based positions the boxes' ids use.
func TestFlowConnectorsPointAtRenderedBoxes(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		flow := parseFlow(t, string(RenderFlow(r)))
		boxes := make(map[int]bool, len(flow.boxes))
		for _, box := range flow.boxes {
			boxes[box.pos] = true
		}
		for _, box := range flow.boxes {
			for _, arrow := range box.arrows {
				if arrow.to != box.pos {
					t.Errorf("%s: connector to %d in box %d; want the box's own position", name, arrow.to, box.pos)
				}
				if arrow.from == 0 {
					continue // dangling reference (W2): no source box exists
				}
				if !boxes[arrow.from] {
					t.Errorf("%s: connector in box %d points from %d, which is not a box", name, box.pos, arrow.from)
				}
				if arrow.from == arrow.to && !hasSelfReference(r, box.pos) {
					t.Errorf("%s: box %d has a self connector it did not write", name, box.pos)
				}
			}
		}
	}
}

// hasSelfReference reports whether the step at pos references its own number.
func hasSelfReference(r *Recipe, pos int) bool {
	if pos < 1 || pos > len(r.Steps) {
		return false
	}
	step := r.Steps[pos-1]
	for _, item := range step.Items {
		if item.Ref > 0 && item.Ref == step.Number {
			return true
		}
	}
	return false
}

// TestFlowDegenerateInput renders documents the parser can produce but the
// canonical fixtures do not: a dangling reference, duplicated step numbers, a
// step with no items, a self reference and an empty verb. None may panic, and
// each must still render a coherent box.
func TestFlowDegenerateInput(t *testing.T) {
	t.Run("dangling ref", func(t *testing.T) {
		fragment := string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Mix", Items: []Item{{Ref: 9}}}}}))
		flow := parseFlow(t, fragment)
		if len(flow.boxes) != 1 || len(flow.boxes[0].arrows) != 1 {
			t.Fatalf("boxes = %+v, want one box with one connector", flow.boxes)
		}
		arrow := flow.boxes[0].arrows[0]
		if !arrow.unknown || arrow.label != "#9" || arrow.verb != "" || arrow.from != 0 {
			t.Errorf("connector = %+v, want an unknown #9 with no source and no verb", arrow)
		}
		if !strings.Contains(fragment, `trn-flow-arrow-unknown"`) {
			t.Error("dangling reference is not classed as unknown")
		}
		if strings.Contains(fragment, "data-from") {
			t.Error("dangling reference claims a source box")
		}
	})

	t.Run("duplicate step numbers", func(t *testing.T) {
		flow := parseFlow(t, string(RenderFlow(&Recipe{Steps: []Step{
			{Number: 1, Verb: "Chop", Items: []Item{{Text: "onion"}}},
			{Number: 1, Verb: "Fry", Items: []Item{{Ref: 1}}},
		}})))
		if len(flow.boxes) != 2 {
			t.Fatalf("got %d boxes, want 2", len(flow.boxes))
		}
		if flow.boxes[1].num != "[1]" {
			t.Errorf("second box number = %q, want [1]", flow.boxes[1].num)
		}
		if flow.boxes[1].pos != 2 {
			t.Errorf("second box position = %d, want 2 (positions, not written numbers)", flow.boxes[1].pos)
		}
		if arrow := flow.boxes[1].arrows[0]; arrow.unknown || arrow.from != 1 || arrow.label != "[1]" || arrow.verb != "Chop" {
			t.Errorf("connector = %+v, want the first box's [1] Chop", arrow)
		}
	})

	t.Run("step without items", func(t *testing.T) {
		fragment := string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Rest", Items: nil}}}))
		flow := parseFlow(t, fragment)
		if len(flow.boxes) != 1 {
			t.Fatalf("got %d boxes, want 1", len(flow.boxes))
		}
		box := flow.boxes[0]
		if len(box.chips) != 0 || len(box.arrows) != 0 {
			t.Errorf("box = %+v, want a bare header box", box)
		}
		if strings.Contains(fragment, flowChipListStart) || strings.Contains(fragment, `class="trn-flow-arrows"`) {
			t.Error("empty lists are rendered instead of omitted")
		}
	})

	t.Run("self reference", func(t *testing.T) {
		flow := parseFlow(t, string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Spin", Items: []Item{{Ref: 1}}}}})))
		if arrow := flow.boxes[0].arrows[0]; arrow.unknown || arrow.from != 1 || arrow.to != 1 || arrow.label != "[1]" || arrow.verb != "Spin" {
			t.Errorf("connector = %+v, want the box's own [1] Spin", arrow)
		}
	})

	t.Run("empty verb", func(t *testing.T) {
		fragment := string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "", Items: []Item{{Text: "salt"}}}}}))
		flow := parseFlow(t, fragment)
		if len(flow.boxes) != 1 || flow.boxes[0].verb != "" || flow.boxes[0].num != "[1]" {
			t.Errorf("boxes = %+v, want one box numbered [1] with an empty verb", flow.boxes)
		}
		if !strings.Contains(fragment, `<span class="trn-flow-verb"></span>`) {
			t.Error("empty verb span is missing")
		}
	})
}

// TestRenderFlowNilAndEmpty pins the degenerate whole-document cases: no panic,
// no error, and a shell rather than markup invented from nothing.
func TestRenderFlowNilAndEmpty(t *testing.T) {
	if got := RenderFlow(nil); got != "" {
		t.Errorf("RenderFlow(nil) = %q, want empty", got)
	}
	empty := string(RenderFlow(&Recipe{}))
	if !strings.Contains(empty, `<figure class="trn-flow">`) {
		t.Error("empty recipe does not render the fragment shell")
	}
	if !strings.Contains(empty, `<ol class="trn-flow-steps">`) {
		t.Error("empty recipe does not render the step list")
	}
	if strings.Contains(empty, `<li class="trn-flow-step`) {
		t.Error("empty recipe rendered a step box")
	}
	if strings.Contains(empty, "<figcaption") {
		t.Error("titleless recipe rendered a figcaption")
	}
	flow := parseFlow(t, string(RenderFlow(&Recipe{Title: "Only a title"})))
	if flow.title != "" || len(flow.boxes) != 0 {
		t.Errorf("default title-only recipe = %+v, want hidden title and no boxes", flow)
	}
	titledFlow := parseFlow(t, string(RenderFlowWithTitle(&Recipe{Title: "Only a title"}, true)))
	if titledFlow.title != "Only a title" || len(titledFlow.boxes) != 0 {
		t.Errorf("titled title-only recipe = %+v, want the title and no boxes", titledFlow)
	}
}

// TestRenderFlowEscaping pushes markup, quotes and Unicode through every text
// field of the flow view and checks that the user's bytes come back intact and
// never become live markup (SPEC-ENGINE §10).
func TestRenderFlowEscaping(t *testing.T) {
	title := `T <script>alert("x")</script> & co`
	verb := `Stir <b>well</b> & "fold" — ünïcode`
	ingredient := `a < 1 && "2" + <img src=x onerror=alert(1)>`
	refVerb := `<i>referenced</i>`
	r := &Recipe{
		Title: title,
		Steps: []Step{
			{Number: 1, Verb: refVerb, Items: []Item{{Text: ingredient}}},
			{Number: 2, Verb: verb, Items: []Item{{Ref: 1}, {Text: ingredient}}},
		},
	}
	fragment := string(RenderFlowWithTitle(r, true))
	// User text legitimately keeps sequences like "onerror=" — it is inert as
	// escaped text — so only real and tag-shaped markup is checked here.
	for _, raw := range []string{"<script>", "<b>well</b>", "<img ", "<i>referenced</i>"} {
		if strings.Contains(fragment, raw) {
			t.Errorf("live markup %q reached the fragment", raw)
		}
	}
	if !strings.Contains(fragment, "&lt;script&gt;") {
		t.Error("title was not HTML-escaped")
	}
	flow := parseFlow(t, fragment)
	if flow.title != title {
		t.Errorf("title round-trip = %q, want %q", flow.title, title)
	}
	if got := flow.boxes[0].verb; got != refVerb {
		t.Errorf("verb round-trip = %q, want %q", got, refVerb)
	}
	if got := flow.boxes[0].chips; len(got) != 1 || got[0] != ingredient {
		t.Errorf("ingredient round-trip = %q, want %q", got, ingredient)
	}
	if got := flow.boxes[1].verb; got != verb {
		t.Errorf("second verb round-trip = %q, want %q", got, verb)
	}
	if got := flow.boxes[1].arrows[0].verb; got != refVerb {
		t.Errorf("connector verb round-trip = %q, want %q", got, refVerb)
	}
	// The escaping must be exactly HTML escaping: unescaping the fragment's
	// text nodes has to reproduce the input byte for byte.
	if html.UnescapeString("&lt;script&gt;") != "<script>" {
		t.Fatal("test's own unescape assertion is broken")
	}
}

// TestRenderFlowPureHTML is acceptance criterion 2 for the flow view: the
// fragment is classed structure only — no stylesheet, no script, no SVG, no
// canvas, no image, no view toggle, no external resource — for every fixture
// and for degenerate input.
func TestRenderFlowPureHTML(t *testing.T) {
	forbidden := []string{
		"<style", "</style", "<script", "</script", "<link", "<svg", "<canvas",
		"<img", "<iframe", "<object", "<embed", "<video", "<audio", "<picture",
		"src=", "srcset=", "http://", "https://", "@import", "@media", "@font-face",
		"url(", "javascript:", "--trn-", "onload=", "onclick=", "onerror=",
		"onmouseover=", "onfocus=", `type="radio"`,
	}
	names := append(fixtureNames(t), "")
	for _, name := range names {
		var fragment string
		if name == "" {
			fragment = string(RenderFlow(&Recipe{Title: "Empty", Steps: []Step{{Number: 1, Verb: "Mix", Items: []Item{{Text: "water"}}}}}))
		} else {
			fragment = string(RenderFlow(loadRecipe(t, name)))
		}
		for _, bad := range forbidden {
			if strings.Contains(fragment, bad) {
				t.Errorf("%s: flow fragment contains %q; the flow view is pure HTML (SPEC-ENGINE §4.2)", name, bad)
			}
		}
		for _, want := range []string{`<figure class="trn-flow">`, `<ol class="trn-flow-steps">`} {
			if !strings.Contains(fragment, want) {
				t.Errorf("%s: flow fragment is missing %q", name, want)
			}
		}
	}
}

// TestFlowChipsAreAList checks that the chip container is a real list — §4.2
// asks for "ingredient chips (a <ul> of chips)" — that every chip sits inside
// one, and that box headers are real headings rather than styled divs.
func TestFlowChipsAreAList(t *testing.T) {
	fragment := string(RenderFlow(loadRecipe(t, "chai.trn.md")))
	// Chai steps 1–3 each introduce ingredient text; steps 4 and 5 introduce
	// nothing, so they have no chip list at all.
	if got := strings.Count(fragment, flowChipListStart); got != 3 {
		t.Errorf("%d chip lists, want 3 (one per chai step with a new ingredient)", got)
	}
	if got := strings.Count(fragment, `<h3 class="trn-flow-head">`); got != 5 {
		t.Errorf("%d box headings, want 5", got)
	}
	if strings.Contains(fragment, `<div class="trn-flow-head`) {
		t.Error("box header is not a heading element")
	}
	total := len(flowChipPattern.FindAllString(fragment, -1))
	inLists := 0
	for _, block := range strings.Split(fragment, flowChipListStart)[1:] {
		inner := block
		if end := strings.Index(inner, "</ul>"); end >= 0 {
			inner = inner[:end]
		}
		inLists += len(flowChipPattern.FindAllString(inner, -1))
	}
	if total == 0 || inLists != total {
		t.Errorf("%d chips, %d of them inside a chip list; every chip belongs to a <ul>", total, inLists)
	}
}
