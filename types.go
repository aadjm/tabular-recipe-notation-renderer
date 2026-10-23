// Package trn parses the frozen v1 TRN (Tabular Recipe Notation) dialect and
// renders it as standalone HTML: a Cooking-for-Engineers-style ingredient/step
// matrix and a flow diagram.
//
// The package is pure by design: stdlib only, no network, no filesystem access,
// no state. Parsing never fails — malformed documents degrade to warnings (see
// the W* codes in parse.go), never to a panic or an error.
//
// The dialect and the exact library API are specified in SPEC-ENGINE.md §2 and
// §5 of the trn repository; the "Decisions" section of this repo's README
// records the choices made where the written rules leave room for
// interpretation.
package trn

// Recipe is a parsed TRN document.
type Recipe struct {
	Title string
	Steps []Step
}

// Step is one numbered action of a recipe.
type Step struct {
	Number int
	Verb   string
	Items  []Item // order preserved
}

// Item is either a reference to an earlier step or an opaque ingredient string.
type Item struct {
	Ref  int // >0 → step reference; 0 → ingredient text
	Text string
}

// Warning is a non-fatal annotation about a malformed or unusual document.
type Warning struct {
	Code string // W1..W7
	Msg  string
}

// RenderOptions configures view selection and title visibility for rendering.
// By default (zero value), View defaults to ViewBoth and ShowTitle is false
// (titles are omitted/invisible unless explicitly enabled).
type RenderOptions struct {
	View      string // ViewTable ("table"), ViewFlow ("flow"), or ViewBoth ("both")
	ShowTitle bool   // when true, render the recipe title in the output
}
