package trn

import (
	"bytes"
	"html/template"
	"strconv"
)

// RenderFlow renders a recipe as the flow view (SPEC-ENGINE §4.2): one box per
// step in document order, numbered, with the verb as the box header; the
// ingredients a step introduces as chips on that box; one connector element per
// reference, pointing from the referenced step's box to the box that draws on
// it; and the `trn-flow-final` class on the final step's box.
//
// The output is structure only: no <style>, no <script>, no SVG, no canvas, no
// image, no view toggle, no external resource. Styling and the view switcher
// belong to the embedding site (SPEC-ENGINE §4.3); the `trn-*` class names are
// the contract it styles against. Every user string goes through
// html/template's escaping paths, so ingredient text keeps its bytes and
// differs only by HTML escaping (SPEC-ENGINE §10).
//
// RenderFlow never panics and never errors: a nil recipe renders nothing, a
// document without steps renders the empty list.
// RenderFlow renders a recipe as the flow view with the title hidden by default.
// Use RenderFlowWithTitle(r, true) or RenderWithOptions to include a visible
// <figcaption> title.
func RenderFlow(r *Recipe) template.HTML {
	return RenderFlowWithTitle(r, false)
}

// RenderFlowWithTitle renders the flow view, optionally including the recipe
// title as a <figcaption> when showTitle is true.
func RenderFlowWithTitle(r *Recipe, showTitle bool) template.HTML {
	if r == nil {
		return ""
	}
	view := buildFlow(r)
	if !showTitle {
		view.Title = ""
	}
	var buf bytes.Buffer
	if err := flowTemplate.Execute(&buf, view); err != nil {
		// Unreachable: the template is a compile-time constant and the data is
		// plain strings and ints. Degrade to an empty fragment rather than
		// panic.
		return ""
	}
	// Safe by construction: every byte of user text in buf was written by
	// html/template's escaper; the rest is this package's own markup.
	return template.HTML(buf.String())
}

// flowView is the whole data set handed to the flow template.
type flowView struct {
	Title string
	Steps []flowStep // one box per step, document order
}

// flowStep is one box of the flow view.
//
// Pos is the step's 1-based position in the document, not its written number:
// it is what the box's id and the connectors' data-from/data-to reference, so
// it stays unique — and every connector has a real target — even when step
// numbers are duplicated or gapped, which the dialect reports as W1 rather than
// rejecting.
type flowStep struct {
	Pos    int
	Number int
	Verb   string
	Final  bool     // the last step in document order: carries trn-flow-final
	Chips  []string // ingredients whose first appearance is this step, in order
	Arrows []flowArrow
}

// flowArrow is one reference drawn as a connector into the box that writes it,
// coming from the box of the step it names.
//
// From is 0 for a reference to a step that does not exist (reported as W2): the
// arrow is still emitted, classed as unknown, carrying the written number only —
// there is no source box to point from.
type flowArrow struct {
	From  int    // 1-based document position of the referenced step; 0 = dangling
	To    int    // 1-based document position of the box that writes the reference
	Label string // "[n]" for a resolved reference, "#n" for a dangling one
	Verb  string // the referenced step's verb; empty when dangling
}

// buildFlow derives the flow model from the AST.
//
// A chip belongs to the step where the ingredient first appears in document
// order (SPEC-ENGINE §4.2); a later listing of the same text is not a second
// chip. References resolve through firstByNumber — the same rule the matrix
// uses — so the views never disagree about which step a `#n` means.
func buildFlow(r *Recipe) flowView {
	view := flowView{Title: r.Title}
	firstAt := firstByNumber(r)
	introduced := make(map[string]bool)

	for i, step := range r.Steps {
		box := flowStep{
			Pos:    i + 1,
			Number: step.Number,
			Verb:   step.Verb,
			Final:  i == len(r.Steps)-1,
		}
		for _, item := range step.Items {
			switch {
			case item.Ref > 0:
				written := strconv.Itoa(item.Ref)
				arrow := flowArrow{To: i + 1, Label: "#" + written}
				if j, ok := firstAt[item.Ref]; ok {
					arrow.From = j + 1
					arrow.Label = "[" + written + "]"
					arrow.Verb = r.Steps[j].Verb
				}
				box.Arrows = append(box.Arrows, arrow)
			case item.Text != "" && !introduced[item.Text]:
				introduced[item.Text] = true
				box.Chips = append(box.Chips, item.Text)
			}
		}
		view.Steps = append(view.Steps, box)
	}
	return view
}

// flowTemplate emits the flow fragment. Literal markup lives here; every piece
// of user text is an action, so html/template escapes it in the right context.
// There is no stylesheet, no script and no toggle in the output.
//
// Geometry is the site's problem: a connector between two boxes is one element
// (`trn-flow-arrow`) that names both ends by 1-based document position — the
// same number as the boxes' `id="trn-flow-step-N"`. The element carries the
// referenced step's number and verb as text, so the relationship survives with
// no CSS at all and assistive technology reads it, while a site with a real
// layout can draw the line from the attributes. Nothing here depends on the two
// boxes being adjacent, which is why the line itself is not baked in: the gap
// between arbitrary boxes depends on font size, wrapping and the heights of the
// boxes in between.
var flowTemplate = template.Must(template.New("trn.flow").Parse(`<figure class="trn-flow">
{{- if .Title}}<figcaption class="trn-title trn-flow-title">{{.Title}}</figcaption>{{end}}
<ol class="trn-flow-steps">
{{- range .Steps}}<li class="trn-flow-step{{if .Final}} trn-flow-final{{end}}" id="trn-flow-step-{{.Pos}}">
<div class="trn-flow-box">
<h3 class="trn-flow-head"><span class="trn-flow-num">[{{.Number}}]</span> <span class="trn-flow-verb">{{.Verb}}</span></h3>
{{- if .Chips}}<ul class="trn-flow-chips">{{range .Chips}}<li class="trn-chip">{{.}}</li>{{end}}</ul>{{end}}
{{- if .Arrows}}<p class="trn-flow-arrows">{{range .Arrows}}<span class="trn-flow-arrow{{if not .From}} trn-flow-arrow-unknown{{end}}"{{if .From}} data-from="{{.From}}"{{end}} data-to="{{.To}}"><span class="trn-flow-arrow-num">{{.Label}}</span>{{if .Verb}} <span class="trn-flow-arrow-verb">{{.Verb}}</span>{{end}}</span>{{end}}</p>{{end}}
</div>
</li>
{{end -}}
</ol>
</figure>
`))
