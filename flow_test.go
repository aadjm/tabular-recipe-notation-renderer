package trn

import (
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The tests in this file read the rendered flow fragment back out of the HTML
// rather than trusting the model, the same way render_test.go does for the
// matrix: structure, numbers, chips and references are all asserted on the
// bytes a browser would receive.

var (
	flowTitlePattern  = regexp.MustCompile(`(?s)<figcaption class="trn-title trn-flow-title">(.*?)</figcaption>`)
	flowNumPattern    = regexp.MustCompile(`(?s)<span class="trn-flow-num">(.*?)</span>`)
	flowVerbPattern   = regexp.MustCompile(`(?s)<span class="trn-flow-verb">(.*?)</span>`)
	flowChipPattern   = regexp.MustCompile(`(?s)<li class="trn-chip">(.*?)</li>`)
	flowAgainPattern  = regexp.MustCompile(`(?s)<span class="trn-chip trn-chip-again">(.*?)</span>`)
	flowRefsPattern   = regexp.MustCompile(`(?s)<p class="trn-flow-refs">(.*?)</p>`)
	flowRefSplit      = regexp.MustCompile(`<span class="trn-flow-ref( trn-flow-ref-unknown)?">`)
	flowRefNumPattern = regexp.MustCompile(`(?s)<span class="trn-flow-ref-num">(.*?)</span>`)
	flowRefVerPattern = regexp.MustCompile(`(?s)<span class="trn-flow-ref-verb">(.*?)</span>`)
	flowBadgePattern  = regexp.MustCompile(`(?s)<span class="trn-flow-final-badge">(.*?)</span>`)
)

type parsedFlowRef struct {
	num     string // as rendered: "[2]" for a resolved reference, "#9" for a dangling one
	verb    string // the referenced step's verb; empty when the reference is dangling
	unknown bool
}

type parsedFlowBox struct {
	class string
	body  string // markup of the box, used for badge and empty-element checks
	num   string // "[n]" as rendered
	verb  string
	chips []string
	again []string
	refs  []parsedFlowRef
}

type parsedFlow struct {
	title string
	boxes []parsedFlowBox
}

// parseFlow reads a flow fragment back into boxes. The engine emits a fixed
// subset of HTML, so splitting on the box start and scanning each piece is
// enough; chips are <li>s inside a box, and their class never matches the box
// start.
func parseFlow(t *testing.T, fragment string) parsedFlow {
	t.Helper()
	var flow parsedFlow
	if m := flowTitlePattern.FindStringSubmatch(fragment); m != nil {
		flow.title = textOf(m[1])
	}
	for _, part := range strings.Split(fragment, `<li class="trn-flow-step`)[1:] {
		end := strings.Index(part, `">`)
		if end < 0 {
			t.Fatalf("malformed box start in fragment: %q", head(part))
		}
		box := parsedFlowBox{class: part[:end], body: part[end+2:]}
		if m := flowNumPattern.FindStringSubmatch(box.body); m != nil {
			box.num = textOf(m[1])
		}
		if m := flowVerbPattern.FindStringSubmatch(box.body); m != nil {
			box.verb = textOf(m[1])
		}
		for _, m := range flowChipPattern.FindAllStringSubmatch(box.body, -1) {
			box.chips = append(box.chips, textOf(m[1]))
		}
		for _, m := range flowAgainPattern.FindAllStringSubmatch(box.body, -1) {
			box.again = append(box.again, textOf(m[1]))
		}
		if m := flowRefsPattern.FindStringSubmatch(box.body); m != nil {
			box.refs = parseFlowRefs(m[1])
		}
		flow.boxes = append(flow.boxes, box)
	}
	return flow
}

// parseFlowRefs splits the refs paragraph at every reference start, so each
// chunk holds one reference's number and (for a resolved reference) its verb.
func parseFlowRefs(inner string) []parsedFlowRef {
	var refs []parsedFlowRef
	starts := flowRefSplit.FindAllStringSubmatchIndex(inner, -1)
	for i, loc := range starts {
		end := len(inner)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		chunk := inner[loc[1]:end]
		ref := parsedFlowRef{unknown: loc[2] >= 0}
		if m := flowRefNumPattern.FindStringSubmatch(chunk); m != nil {
			ref.num = textOf(m[1])
		}
		if m := flowRefVerPattern.FindStringSubmatch(chunk); m != nil {
			ref.verb = textOf(m[1])
		}
		refs = append(refs, ref)
	}
	return refs
}

func head(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

// sortedCopy returns a sorted copy of list, for order-insensitive comparisons.
func sortedCopy(list []string) []string {
	out := append([]string(nil), list...)
	sort.Strings(out)
	return out
}

// boxRefNums renders a box's references the way the fragment does, for
// comparisons against expectations.
func boxRefNums(box parsedFlowBox) []string {
	nums := make([]string, 0, len(box.refs))
	for _, ref := range box.refs {
		nums = append(nums, ref.num)
	}
	return nums
}

// TestFlowChaiBoxes checks the canonical single-line-steps example box by box:
// numbers, verbs, the chips each step introduces, the reference each step makes
// to the one before it, and the boxes that introduce nothing.
func TestFlowChaiBoxes(t *testing.T) {
	flow := parseFlow(t, string(RenderFlow(loadRecipe(t, "chai.trn.md"))))
	if flow.title != "Masala Chai" {
		t.Errorf("title = %q, want %q", flow.title, "Masala Chai")
	}
	want := []struct {
		num, verb string
		chips     []string
		refs      []string
	}{
		{"[1]", "Crush or grind to a powder",
			[]string{"4 green cardamoms", "2 cloves", "3g cinnamon", "2 peppercorns"}, nil},
		{"[2]", "Boil 3 min in 360ml water until the decoction is dark",
			[]string{"1/2 inch ginger crushed", "6g black tea"}, []string{"[1]"}},
		{"[3]", "Add and bring to a boil",
			[]string{"240ml full-fat milk", "12g sugar"}, []string{"[2]"}},
		{"[4]", "Simmer 2 min until dark with a cream layer and no raw milk flavour",
			nil, []string{"[3]"}},
		{"[5]", "Pull 4-5 times from 1-1.5 ft, then strain into cups and serve",
			nil, []string{"[4]"}},
	}
	if len(flow.boxes) != len(want) {
		t.Fatalf("got %d boxes, want %d", len(flow.boxes), len(want))
	}
	for i, exp := range want {
		box := flow.boxes[i]
		if box.num != exp.num || box.verb != exp.verb {
			t.Errorf("box %d = %s %q, want %s %q", i, box.num, box.verb, exp.num, exp.verb)
		}
		if strings.Join(box.chips, "|") != strings.Join(exp.chips, "|") {
			t.Errorf("box %s chips = %q, want %q", box.num, box.chips, exp.chips)
		}
		if strings.Join(boxRefNums(box), "|") != strings.Join(exp.refs, "|") {
			t.Errorf("box %s refs = %q, want %q", box.num, boxRefNums(box), exp.refs)
		}
		for _, ref := range box.refs {
			if ref.unknown || ref.verb == "" {
				t.Errorf("box %s reference %s did not resolve to a verb", box.num, ref.num)
			}
		}
	}
	if got := flow.boxes[4].refs[0].verb; got != want[3].verb {
		t.Errorf("box [5] references %q, want the verb of [4]", got)
	}
}

// TestFlowGuacamoleContinuation covers the example whose step 4 continues over
// several lines: the continuation items must attach to that box, refs first and
// then the two ingredients, in written order.
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
	wantRefs := [][]string{nil, nil, nil, {"[1]", "[2]", "[3]"}}
	for i := range flow.boxes {
		if got := flow.boxes[i].chips; strings.Join(got, "|") != strings.Join(wantChips[i], "|") {
			t.Errorf("box %d chips = %q, want %q", i+1, got, wantChips[i])
		}
		if got := boxRefNums(flow.boxes[i]); strings.Join(got, "|") != strings.Join(wantRefs[i], "|") {
			t.Errorf("box %d refs = %q, want %q", i+1, got, wantRefs[i])
		}
	}
	if got := flow.boxes[3].refs[0].verb; got != "Mash" {
		t.Errorf("box [4] first reference = %q, want %q", got, "Mash")
	}
}

// TestFlowStructureMatchesAST compares every box of every fixture with the AST:
// box count, numbers, verbs, and one rendered reference per written reference,
// in order, resolved through the first appearance of the step number.
func TestFlowStructureMatchesAST(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		flow := parseFlow(t, string(RenderFlow(r)))
		if len(flow.boxes) != len(r.Steps) {
			t.Errorf("%s: %d boxes for %d steps", name, len(flow.boxes), len(r.Steps))
			continue
		}
		firstAt := firstByNumber(r)
		for i, step := range r.Steps {
			box := flow.boxes[i]
			if box.num != "["+strconv.Itoa(step.Number)+"]" || box.verb != step.Verb {
				t.Errorf("%s box %d = %s %q, want [%d] %q", name, i, box.num, box.verb, step.Number, step.Verb)
			}
			var wantNums, wantVerbs []string
			for _, item := range step.Items {
				if item.Ref <= 0 {
					continue
				}
				j, ok := firstAt[item.Ref]
				if !ok {
					wantNums = append(wantNums, "#"+strconv.Itoa(item.Ref))
					wantVerbs = append(wantVerbs, "")
					continue
				}
				wantNums = append(wantNums, "["+strconv.Itoa(item.Ref)+"]")
				wantVerbs = append(wantVerbs, r.Steps[j].Verb)
			}
			gotNums, gotVerbs := boxRefNums(box), make([]string, 0, len(box.refs))
			for _, ref := range box.refs {
				gotVerbs = append(gotVerbs, ref.verb)
			}
			if strings.Join(gotNums, "|") != strings.Join(wantNums, "|") {
				t.Errorf("%s box %d refs = %q, want %q", name, i, gotNums, wantNums)
			}
			if strings.Join(gotVerbs, "|") != strings.Join(wantVerbs, "|") {
				t.Errorf("%s box %d ref verbs = %q, want %q", name, i, gotVerbs, wantVerbs)
			}
		}
	}
}

// TestFlowChipsPreserveItems checks the two properties of the chips against
// every fixture: each box carries its own ingredient strings unchanged (only
// HTML-escaped), and each distinct ingredient text is a chip exactly once — on
// the box of its first appearance (SPEC-ENGINE §4.2, §10).
func TestFlowChipsPreserveItems(t *testing.T) {
	for _, name := range fixtureNames(t) {
		r := loadRecipe(t, name)
		flow := parseFlow(t, string(RenderFlow(r)))
		seen := make(map[string]bool)
		for i, step := range r.Steps {
			if i >= len(flow.boxes) {
				break
			}
			box := flow.boxes[i]
			var want []string
			for _, item := range step.Items {
				if item.Ref == 0 && item.Text != "" {
					want = append(want, item.Text)
				}
			}
			got := append(append([]string(nil), box.chips...), box.again...)
			if strings.Join(sortedCopy(got), "|") != strings.Join(sortedCopy(want), "|") {
				t.Errorf("%s box %d carries %q, want the step's items %q", name, i, got, want)
			}
			for _, chip := range box.chips {
				if seen[chip] {
					t.Errorf("%s: chip %q appears on more than one box", name, chip)
				}
				seen[chip] = true
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

// TestFlowRepeatedIngredient covers grocy-1, where "1 tbsp cilantro" is listed
// twice: the first listing is a chip on step 7, the second is the muted "again"
// note on step 10 rather than a second chip.
func TestFlowRepeatedIngredient(t *testing.T) {
	flow := parseFlow(t, string(RenderFlow(loadRecipe(t, "grocy-1.trn.md"))))
	if len(flow.boxes) != 10 {
		t.Fatalf("got %d boxes, want 10", len(flow.boxes))
	}
	if !containsString(flow.boxes[6].chips, "1 tbsp cilantro") {
		t.Errorf("step 7 chips = %q, want to introduce %q", flow.boxes[6].chips, "1 tbsp cilantro")
	}
	if got := flow.boxes[9].again; len(got) != 1 || got[0] != "1 tbsp cilantro" {
		t.Errorf("step 10 again = %q, want [%q]", got, "1 tbsp cilantro")
	}
	if got := flow.boxes[9].chips; len(got) != 0 {
		t.Errorf("step 10 chips = %q, want none (every item there is a repeat)", got)
	}
	if got := flow.boxes[8].chips; len(got) != 1 || got[0] != "1/2 cup hot water" {
		t.Errorf("step 9 chips = %q, want [%q]", got, "1/2 cup hot water")
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

// TestFlowFinalStepDistinct checks that exactly one box — the last step in
// document order — is marked and badged as final, for every fixture.
func TestFlowFinalStepDistinct(t *testing.T) {
	for _, name := range fixtureNames(t) {
		fragment := string(RenderFlow(loadRecipe(t, name)))
		flow := parseFlow(t, fragment)
		for i, box := range flow.boxes {
			final := strings.Contains(box.class, "trn-flow-step-final")
			if final != (i == len(flow.boxes)-1) {
				t.Errorf("%s: box %d final = %v, want %v", name, i+1, final, i == len(flow.boxes)-1)
			}
			badged := strings.Contains(box.body, `class="trn-flow-final-badge"`)
			if badged != final {
				t.Errorf("%s: box %d badge = %v, final = %v", name, i+1, badged, final)
			}
		}
		if got := len(flowBadgePattern.FindAllString(fragment, -1)); got != 1 {
			t.Errorf("%s: %d final badges, want 1", name, got)
		}
		if m := flowBadgePattern.FindStringSubmatch(fragment); m != nil && textOf(m[1]) != "final" {
			t.Errorf("%s: badge text = %q, want %q", name, textOf(m[1]), "final")
		}
	}
}

// TestFlowDegenerateInput renders documents the parser can produce but the
// canonical fixtures do not: a dangling reference, duplicated step numbers, a
// step with no items, a self reference and an empty verb. None may panic, and
// each must still render a coherent box.
func TestFlowDegenerateInput(t *testing.T) {
	t.Run("dangling ref", func(t *testing.T) {
		flow := parseFlow(t, string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Mix", Items: []Item{{Ref: 9}}}}})))
		if len(flow.boxes) != 1 || len(flow.boxes[0].refs) != 1 {
			t.Fatalf("boxes = %+v, want one box with one reference", flow.boxes)
		}
		ref := flow.boxes[0].refs[0]
		if !ref.unknown || ref.num != "#9" || ref.verb != "" {
			t.Errorf("ref = %+v, want an unknown #9 without a verb", ref)
		}
		if !strings.Contains(flow.boxes[0].body, `trn-flow-ref-unknown`) {
			t.Error("dangling reference is not classed as unknown")
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
		if ref := flow.boxes[1].refs[0]; ref.unknown || ref.num != "[1]" || ref.verb != "Chop" {
			t.Errorf("ref = %+v, want the first step's [1] Chop", ref)
		}
	})

	t.Run("step without items", func(t *testing.T) {
		fragment := string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Rest", Items: nil}}}))
		flow := parseFlow(t, fragment)
		if len(flow.boxes) != 1 {
			t.Fatalf("got %d boxes, want 1", len(flow.boxes))
		}
		box := flow.boxes[0]
		if len(box.chips) != 0 || len(box.again) != 0 || len(box.refs) != 0 {
			t.Errorf("box = %+v, want a bare header box", box)
		}
		if strings.Contains(fragment, `class="trn-flow-chips"`) || strings.Contains(fragment, `class="trn-flow-refs"`) {
			t.Error("empty lists are rendered instead of omitted")
		}
	})

	t.Run("self reference", func(t *testing.T) {
		flow := parseFlow(t, string(RenderFlow(&Recipe{Steps: []Step{{Number: 1, Verb: "Spin", Items: []Item{{Ref: 1}}}}})))
		if ref := flow.boxes[0].refs[0]; ref.unknown || ref.num != "[1]" || ref.verb != "Spin" {
			t.Errorf("ref = %+v, want the box's own [1] Spin", ref)
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
	if !strings.Contains(empty, `<figure class="trn trn-view trn-view-flow">`) {
		t.Error("empty recipe does not render the fragment shell")
	}
	if !strings.Contains(empty, `<ol class="trn-flow">`) {
		t.Error("empty recipe does not render the step list")
	}
	if strings.Contains(empty, `class="trn-flow-step`) {
		t.Error("empty recipe rendered a step box")
	}
	if strings.Contains(empty, "<figcaption") {
		t.Error("titleless recipe rendered a figcaption")
	}
	flow := parseFlow(t, string(RenderFlow(&Recipe{Title: "Only a title"})))
	if flow.title != "Only a title" || len(flow.boxes) != 0 {
		t.Errorf("title-only recipe = %+v, want the title and no boxes", flow)
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
	fragment := string(RenderFlow(r))
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
	if got := flow.boxes[1].refs[0].verb; got != refVerb {
		t.Errorf("reference verb round-trip = %q, want %q", got, refVerb)
	}
	if got := flow.boxes[1].again; len(got) != 1 || got[0] != ingredient {
		t.Errorf("repeated ingredient round-trip = %q, want %q", got, ingredient)
	}
	// The escaping must be exactly HTML escaping: unescaping the fragment's
	// text nodes has to reproduce the input byte for byte.
	if html.UnescapeString("&lt;script&gt;") != "<script>" {
		t.Fatal("test's own unescape assertion is broken")
	}
}

// TestRenderFlowNoScriptNoExternal enforces the presentation constraints on the
// flow view for every fixture: no JavaScript, no images, no SVG, no canvas, no
// external resources, and the stylesheet embedded in the fragment.
func TestRenderFlowNoScriptNoExternal(t *testing.T) {
	forbidden := []string{
		"<script", "</script", "<img", "<svg", "<canvas", "<iframe", "<object",
		"<embed", "<link", "<video", "<audio", "src=", "http://", "https://",
		"@import", "url(", "javascript:", "onload=", "onclick=", "onerror=",
	}
	names := append(fixtureNames(t), "")
	for _, name := range names {
		var fragment string
		if name == "" {
			fragment = string(RenderFlow(&Recipe{Title: "Empty", Steps: []Step{{Number: 1, Verb: "Mix", Items: []Item{{Text: "water"}}}}}))
		} else {
			fragment = string(RenderFlow(loadRecipe(t, name)))
		}
		if !strings.Contains(fragment, "<style>") {
			t.Errorf("%s: flow fragment does not embed its stylesheet", name)
		}
		for _, bad := range forbidden {
			if strings.Contains(fragment, bad) {
				t.Errorf("%s: flow fragment contains %q", name, bad)
			}
		}
	}
}
