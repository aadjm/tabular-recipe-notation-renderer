package trn

import "fmt"

// W4 is the one warning code the parser cannot decide on its own: whether an
// ingredient ever reaches the final step depends on the whole reference graph
// (SPEC-ENGINE §3, §4.1). The renderers and the embedding site get it from
// ConvergenceWarnings, which reports through the same []Warning channel.

// ConvergenceWarnings reports W4 for every ingredient that never reaches the
// final step, directly or transitively.
//
// The final step is the last step in document order. An ingredient reaches it
// when it is listed directly in that step, or in a step that the final step
// reaches through a chain of back-references (step 4 → #3 → #2 …). A recipe
// with no steps cannot converge on anything and yields no W4 — that document
// is already reported as W5 by Parse.
//
// Ingredient identity is the exact item text: the dialect preserves ingredient
// strings byte-for-byte, so two items are the same ingredient only when their
// bytes match. An ingredient listed more than once reaches the final step if
// any of its occurrences does.
//
// Warnings come back in document order of each ingredient's first appearance.
// They carry code "W4" and are never errors; callers append them to the parser's
// warnings (re-sort the merged slice if the W1 … W7 code order matters).
func ConvergenceWarnings(r *Recipe) []Warning {
	if r == nil || len(r.Steps) == 0 {
		return nil
	}
	reachesFinal := ancestorSteps(r, len(r.Steps)-1)

	// Collect the ingredients in first-appearance order and remember which
	// steps list each of them.
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

	final := r.Steps[len(r.Steps)-1]
	warnings := make([]Warning, 0, len(order))
	for _, text := range order {
		if anyTrue(reachesFinal, listedIn[text]) {
			continue
		}
		warnings = append(warnings, Warning{
			Code: codeFinal,
			Msg:  fmt.Sprintf("ingredient %q never reaches the final step [%d]", text, final.Number),
		})
	}
	if len(warnings) == 0 {
		return nil
	}
	return warnings
}

// ancestorSteps reports which step indices feed step target: target itself plus
// every step it transitively references. The matrix renderer calls it once per
// column (SPEC-ENGINE §4.1).
//
// A reference resolves to the first appearance of its step number, matching the
// parser's rule for duplicated numbers. References to steps that do not exist
// are ignored here — Parse reports them as W2. References are followed as
// written, so a forward reference (W3) or a cycle also contributes a reachable
// step; cycles terminate because each index is visited at most once.
//
// Complexity is O(steps × items) per call: documents are tiny, so the obvious
// graph walk beats anything cleverer.
func ancestorSteps(r *Recipe, target int) []bool {
	if r == nil {
		return nil
	}
	reach := make([]bool, len(r.Steps))
	if target < 0 || target >= len(r.Steps) {
		return reach
	}
	firstAt := firstByNumber(r)

	reach[target] = true
	stack := []int{target}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, item := range r.Steps[i].Items {
			if item.Ref <= 0 {
				continue
			}
			j, ok := firstAt[item.Ref]
			if !ok || reach[j] {
				continue
			}
			reach[j] = true
			stack = append(stack, j)
		}
	}
	return reach
}

// firstByNumber maps every step number to the index of its first appearance,
// which is how references resolve when numbers are duplicated (W1). Shared by
// the reachability walk, the W4 check and the matrix renderer so all three
// agree on which step a reference means.
func firstByNumber(r *Recipe) map[int]int {
	first := make(map[int]int, len(r.Steps))
	for i, step := range r.Steps {
		if _, seen := first[step.Number]; !seen {
			first[step.Number] = i
		}
	}
	return first
}

// anyTrue reports whether any of the given indices is set in flags.
func anyTrue(flags []bool, indices []int) bool {
	for _, i := range indices {
		if flags[i] {
			return true
		}
	}
	return false
}
