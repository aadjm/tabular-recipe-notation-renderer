package trn

import (
	"reflect"
	"strings"
	"testing"
)

// warningsFor gathers the full warning channel for a document: the parse-time
// codes from Parse plus the render-time W4s from ConvergenceWarnings. Every
// caller of the engine is expected to do the same, so the tests exercise the
// channel end to end.
func warningsFor(input string) (*Recipe, []Warning) {
	r, parseWarnings := Parse(input)
	all := make([]Warning, 0, len(parseWarnings)+1)
	all = append(all, parseWarnings...)
	all = append(all, ConvergenceWarnings(r)...)
	return r, all
}

// SPEC-ENGINE §3 / §6: one minimal document per warning code, each producing
// exactly that code and nothing else. W1–W3, W5–W7 come from Parse; W4 comes
// from the render-time reachability pass. The W4 case proves the codes compose:
// neither half reports the other's warnings.
func TestWarningsOnePerCode(t *testing.T) {
	cases := []struct {
		name  string
		code  string
		input string
	}{
		{
			name:  "W1 step numbers not strictly increasing from 1",
			code:  "W1",
			input: "Toast\n[1] A: x\n[3] B: #1\n",
		},
		{
			name:  "W2 reference to a step that does not exist",
			code:  "W2",
			input: "Toast\n[1] A: x, #9\n",
		},
		{
			name:  "W3 forward reference",
			code:  "W3",
			input: "Toast\n[1] A: #2\n[2] B: x\n",
		},
		{
			name:  "W4 ingredient never reaches the final step",
			code:  "W4",
			input: "Toast\n[1] A: x\n[2] B: y\n",
		},
		{
			name:  "W5 document contains no steps",
			code:  "W5",
			input: "Toast\n",
		},
		{
			name:  "W6 empty title",
			code:  "W6",
			input: "[1] A: x\n",
		},
		{
			name:  "W7 step verb is empty",
			code:  "W7",
			input: "Toast\n[1] : x\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := warningsFor(tc.input)
			if got := codes(warnings); !reflect.DeepEqual(got, []string{tc.code}) {
				t.Fatalf("codes = %v, want exactly [%s] (warnings: %v)", got, tc.code, warnings)
			}
			for i, w := range warnings {
				if !validCodes[w.Code] {
					t.Errorf("warning %d has unknown code %q", i, w.Code)
				}
				if strings.TrimSpace(w.Msg) == "" {
					t.Errorf("warning %d (%s) has an empty message", i, w.Code)
				}
			}
		})
	}
}

// W4 is reserved at parse time and reported by the render-time channel: Parse
// alone stays silent about reachability (README decision 8).
func TestW4IsRenderTimeOnly(t *testing.T) {
	r, parseWarnings := Parse("Toast\n[1] A: x\n[2] B: y\n")
	if len(parseWarnings) != 0 {
		t.Fatalf("Parse reported %v, want no parse-time warnings", parseWarnings)
	}
	warnings := ConvergenceWarnings(r)
	if len(warnings) != 1 {
		t.Fatalf("ConvergenceWarnings = %v, want exactly one W4", warnings)
	}
	if warnings[0].Code != "W4" {
		t.Fatalf("code = %q, want W4", warnings[0].Code)
	}
	if !strings.Contains(warnings[0].Msg, `"x"`) {
		t.Errorf("message %q does not name the stranded ingredient", warnings[0].Msg)
	}
}

// ConvergenceWarnings is the detection logic behind W4 (SPEC-ENGINE §4.1). The
// table pins the reachability rules: transitive chains, the final step being
// the last step in document order, first-appearance resolution for duplicated
// step numbers, dangling references, cycles, and repeated ingredients.
func TestConvergenceWarnings(t *testing.T) {
	cases := []struct {
		name  string
		input string
		codes []string // full channel: parse warnings + W4
	}{
		{
			name:  "empty document has nothing to strand",
			input: "",
			codes: []string{"W5", "W6"},
		},
		{
			name:  "single step is trivially final",
			input: "Toast\n[1] A: x, y\n",
			codes: nil,
		},
		{
			name:  "transitive chain converges",
			input: "Toast\n[1] A: x\n[2] B: #1, y\n[3] C: #2\n",
			codes: nil,
		},
		{
			name:  "ingredient stranded in a step the final step ignores",
			input: "Toast\n[1] A: x\n[2] B: y\n[3] C: #2\n",
			codes: []string{"W4"},
		},
		{
			name:  "final step with no items and no refs strands everything",
			input: "Toast\n[1] A: x\n[2] B\n",
			codes: []string{"W4"},
		},
		{
			name:  "ingredient repeated in a reachable step converges",
			input: "Toast\n[1] A: salt\n[2] B: x\n[3] C: #2, salt\n",
			codes: nil,
		},
		{
			name:  "dangling reference does not rescue ingredients",
			input: "Toast\n[1] A: x\n[2] B: #9\n",
			codes: []string{"W2", "W4"},
		},
		{
			name:  "reference cycle behind a forward reference terminates",
			input: "Toast\n[1] A: x, #2\n[2] B: #1\n",
			codes: []string{"W3"},
		},
		{
			name:  "duplicated step numbers resolve to their first appearance",
			input: "Toast\n[1] A: x\n[1] B: y\n[2] C: #1\n",
			codes: []string{"W1", "W1", "W4"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := warningsFor(tc.input)
			if got := codes(warnings); !reflect.DeepEqual(got, tc.codes) {
				t.Fatalf("codes = %v, want %v (warnings: %v)", got, tc.codes, warnings)
			}
		})
	}
}

// ancestorSteps is the primitive the matrix renderer will call per column; the
// convergence rule is its final-step instance.
func TestAncestorSteps(t *testing.T) {
	r, _ := Parse("Toast\n[1] A: x\n[2] B: #1, y\n[3] C: #2\n[4] D: #1\n")

	want := [][]bool{
		{true, false, false, false}, // step 1 feeds itself only
		{true, true, false, false},  // step 2 pulls step 1 in
		{true, true, true, false},   // step 3 pulls steps 2 and 1 in
		{true, false, false, true},  // step 4 pulls step 1 in
	}
	for target := range r.Steps {
		if got := ancestorSteps(r, target); !reflect.DeepEqual(got, want[target]) {
			t.Errorf("ancestorSteps(step %d) = %v, want %v", target+1, got, want[target])
		}
	}

	if got := ancestorSteps(r, -1); !reflect.DeepEqual(got, []bool{false, false, false, false}) {
		t.Errorf("ancestorSteps(-1) = %v, want all false", got)
	}
	if got := ancestorSteps(r, len(r.Steps)); !reflect.DeepEqual(got, []bool{false, false, false, false}) {
		t.Errorf("ancestorSteps(out of range) = %v, want all false", got)
	}
	if got := ancestorSteps(nil, 0); len(got) != 0 {
		t.Errorf("ancestorSteps(nil) = %v, want empty", got)
	}
}

// A nil recipe must not panic: the site may hold a zero value before parsing.
func TestConvergenceWarningsNilRecipe(t *testing.T) {
	if got := ConvergenceWarnings(nil); got != nil {
		t.Errorf("ConvergenceWarnings(nil) = %v, want nil", got)
	}
}
