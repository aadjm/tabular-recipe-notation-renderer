package trn

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Warning codes (SPEC-ENGINE §3). Warnings are never errors: a malformed
// document always yields a usable Recipe.
const (
	codeSequence = "W1" // step numbers not strictly increasing from 1
	codeMissing  = "W2" // reference points to a step that does not exist
	codeForward  = "W3" // reference points to a later step
	codeFinal    = "W4" // ingredient never reaches the final step
	codeNoSteps  = "W5" // document contains no steps
	codeNoTitle  = "W6" // empty title
	codeNoVerb   = "W7" // step verb is empty
)

// stepPattern matches a step line after its leading indentation has been
// removed: "[12] Verb: item, item". The captured rest is split at its first
// colon, so verbs may contain colons only by surrendering the tail as items.
var stepPattern = regexp.MustCompile(`^\[([0-9]+)\]\s*(.*)$`)

// refPattern matches an item that is a back-reference to another step. The
// match must cover the whole (trimmed) item.
var refPattern = regexp.MustCompile(`^#([0-9]+)$`)

// utf8BOM is stripped from the head of the input: it is invisible in editors
// and would otherwise become part of the title.
const utf8BOM = "\ufeff"

// Parse turns a TRN markdown document into its AST plus a list of warnings.
//
// It never returns an error and never panics: every malformed construct in the
// input is reported through the []Warning channel (SPEC-ENGINE §10). Warnings
// are returned in code order (W1 … W7), stable within a code.
func Parse(input string) (*Recipe, []Warning) {
	p := parser{recipe: &Recipe{}}

	input = strings.TrimPrefix(input, utf8BOM)
	for _, line := range strings.Split(input, "\n") {
		line = trimLineEnd(line)
		if line == "" {
			continue // blank lines are ignored
		}
		if !p.titleSeen {
			p.titleSeen = true
			if !stepPattern.MatchString(trimLeftSpace(line)) {
				p.recipe.Title = strings.TrimSpace(line)
				continue
			}
			// The document opens with a step: the title stays empty (W6).
		}
		if left := trimLeftSpace(line); stepPattern.MatchString(left) {
			p.addStep(left)
			continue
		}
		p.addContinuation(line)
	}

	p.checkSequence()
	p.checkRefs()
	if len(p.recipe.Steps) == 0 {
		p.warn(codeNoSteps, "document contains no steps")
	}
	if strings.TrimSpace(p.recipe.Title) == "" {
		p.warn(codeNoTitle, "document has no title")
	}
	sort.SliceStable(p.warnings, func(i, j int) bool {
		return p.warnings[i].Code < p.warnings[j].Code
	})
	return p.recipe, p.warnings
}

type parser struct {
	recipe    *Recipe
	warnings  []Warning
	titleSeen bool
}

func (p *parser) warn(code, format string, args ...any) {
	p.warnings = append(p.warnings, Warning{Code: code, Msg: fmt.Sprintf(format, args...)})
}

// addStep parses a step line whose leading indentation has already been
// stripped. The caller has verified that it matches stepPattern.
func (p *parser) addStep(line string) {
	m := stepPattern.FindStringSubmatch(line)
	if m == nil {
		return // unreachable: callers only pass matching lines
	}
	number, err := strconv.Atoi(m[1])
	if err != nil {
		// More digits than an int can hold. Keep the step, mark the number as
		// unknown (0); checkSequence reports it as W1.
		number = 0
	}

	rest := m[2]
	verb := rest
	itemsRaw := ""
	if idx := strings.Index(rest, ":"); idx >= 0 {
		verb = rest[:idx]
		itemsRaw = rest[idx+1:]
	}
	verb = strings.TrimSpace(verb)
	if verb == "" {
		p.warn(codeNoVerb, "step [%d] has an empty verb", number)
	}
	p.recipe.Steps = append(p.recipe.Steps, Step{
		Number: number,
		Verb:   verb,
		Items:  parseItems(itemsRaw),
	})
}

// addContinuation appends a line's items to the most recent step. Continuation
// lines need not start with whitespace; any non-blank line after the title that
// is not a step line continues the previous step. If no step exists yet, the
// text has nowhere to go and is dropped (documented in docs/DECISIONS.md).
func (p *parser) addContinuation(line string) {
	if len(p.recipe.Steps) == 0 {
		return
	}
	text := strings.TrimSpace(line)
	text = strings.TrimSuffix(text, ",") // strip the trailing comma first
	last := len(p.recipe.Steps) - 1
	p.recipe.Steps[last].Items = append(p.recipe.Steps[last].Items, parseItems(text)...)
}

// parseItems splits a comma-separated item list, trims each item, drops empty
// entries (trailing/doubled commas) and classifies the rest as step references
// or opaque ingredient text. Ingredient text keeps its interior bytes verbatim:
// only the surrounding whitespace is removed.
func parseItems(raw string) []Item {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]Item, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part)
		if text == "" {
			continue
		}
		if m := refPattern.FindStringSubmatch(text); m != nil {
			// "#0" cannot be represented as a reference (Ref 0 is the ingredient
			// sentinel), and digests too large for an int are not references
			// either: both are kept as opaque ingredient text.
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				items = append(items, Item{Ref: n})
				continue
			}
		}
		items = append(items, Item{Text: text})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// checkSequence reports W1 for every step whose written number is not the
// position it occupies (1-based): gaps, duplicates and out-of-order numbers.
func (p *parser) checkSequence() {
	for i, step := range p.recipe.Steps {
		if step.Number != i+1 {
			p.warn(codeSequence, "step %d at position %d is out of sequence (expected %d)", step.Number, i+1, i+1)
		}
	}
}

// checkRefs reports W2 for references to steps that do not exist and W3 for
// references to steps that appear later in the document. A duplicated step
// number resolves to its first appearance.
func (p *parser) checkRefs() {
	firstAt := make(map[int]int, len(p.recipe.Steps))
	for i, step := range p.recipe.Steps {
		if _, seen := firstAt[step.Number]; !seen {
			firstAt[step.Number] = i
		}
	}
	for i, step := range p.recipe.Steps {
		for _, item := range step.Items {
			if item.Ref <= 0 {
				continue
			}
			at, ok := firstAt[item.Ref]
			switch {
			case !ok:
				p.warn(codeMissing, "step [%d] references #%d, which does not exist", step.Number, item.Ref)
			case at > i:
				p.warn(codeForward, "step [%d] references #%d, which appears later", step.Number, item.Ref)
			}
		}
	}
}

// trimLineEnd removes the line terminator and trailing horizontal whitespace,
// which SPEC-ENGINE §2.1 declares insignificant. Interior bytes are untouched.
func trimLineEnd(line string) string {
	return strings.TrimRight(line, " \t\r\v\f")
}

// trimLeftSpace removes leading horizontal whitespace so an indented step line
// is still recognised as a step (the reference implementation indents steps
// under the title; see the Kraft Dinner fixture).
func trimLeftSpace(line string) string {
	return strings.TrimLeft(line, " \t")
}
