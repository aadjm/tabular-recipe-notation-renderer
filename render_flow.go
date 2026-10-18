package trn

import (
	"bytes"
	"html/template"
)

// RenderFlow renders a recipe as the flow view (SPEC-ENGINE §4.2): one box per
// step in document order, numbered, with the verb as the box header; the
// ingredients a step introduces as chips on that box; an arrow per reference
// naming the step it comes from; and a visually distinct box for the final
// step.
//
// The fragment is self-contained, exactly like RenderTable's: classed markup
// (`trn-flow`, `trn-flow-step`, `trn-chip`, `trn-flow-ref`, …), its own
// light/dark stylesheet and a print stylesheet, no JavaScript, no images, no
// external resources — so the embedding site can use it as-is or re-skin it.
//
// All user text (title, verbs, ingredients, reference verbs) goes through
// html/template's escaping paths; ingredient strings keep their bytes and
// differ only by HTML escaping (SPEC-ENGINE §10). RenderFlow never panics and
// never errors: a nil recipe renders nothing, a document without steps renders
// the empty shell.
func RenderFlow(r *Recipe) template.HTML {
	if r == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := flowTemplate.Execute(&buf, buildFlow(r)); err != nil {
		// Unreachable: the template is a compile-time constant and the data is
		// plain strings and ints. Degrade to an empty fragment anyway — this
		// package never turns a rendering problem into a panic.
		return ""
	}
	// Safe by construction: every byte of user text in buf was written by
	// html/template's escaper, the rest is this package's own markup and CSS.
	return template.HTML(buf.String())
}

// flowView is the whole data set handed to the flow template.
type flowView struct {
	Title string
	Steps []flowStep // one box per step, document order
}

// flowStep is one box of the flow view.
type flowStep struct {
	Number int
	Verb   string
	Final  bool     // the last step in document order: rendered distinctly
	Chips  []string // ingredients whose first appearance is this step, in order
	Again  []string // ingredients listed again here, already introduced earlier
	Refs   []stepRef
}

// buildFlow derives the flow model from the AST.
//
// A chip belongs to the step where the ingredient first appears in document
// order; a later listing of the same text is not a new ingredient, so it lands
// in Again. Both lists keep document order, and together they carry every
// ingredient string of the step, so the box shows the same item text the
// document does (only HTML-escaped).
func buildFlow(r *Recipe) flowView {
	view := flowView{Title: r.Title}
	firstAt := firstByNumber(r)
	introduced := make(map[string]bool)

	for i, step := range r.Steps {
		box := flowStep{Number: step.Number, Verb: step.Verb, Final: i == len(r.Steps)-1}
		for _, item := range step.Items {
			switch {
			case item.Ref > 0:
				ref := stepRef{Number: item.Ref}
				if j, ok := firstAt[item.Ref]; ok {
					ref.Known = true
					ref.Verb = r.Steps[j].Verb
				}
				box.Refs = append(box.Refs, ref)
			case item.Text != "":
				if introduced[item.Text] {
					box.Again = append(box.Again, item.Text)
					continue
				}
				introduced[item.Text] = true
				box.Chips = append(box.Chips, item.Text)
			}
		}
		view.Steps = append(view.Steps, box)
	}
	return view
}

// flowTemplate emits the flow fragment. Literal markup and CSS live here; every
// piece of user text is an action, so html/template escapes it in the right
// context.
//
// Geometry: the boxes sit in a single column on a vertical rail, and the rail
// carries a down-arrow into every box after the first — that is the sequence of
// steps. A reference is drawn as an arrow inside the referencing box that names
// the step it comes from (`← [1] Crush or grind to a powder`), because a line
// between two arbitrary — possibly far apart — boxes cannot be positioned in CSS
// without per-edge pixel geometry, which the renderer must not invent (and
// §4.2 forbids the SVG that would make it easy). The glyph is CSS-generated so
// the markup carries no arrows; the word "from" carries the meaning for
// assistive technology, which announces the box, then the steps feeding it.
//
// The final box is distinct by a heavier accent border, a tinted background and
// a "final" badge, so it stays distinct when printed or re-skinned.
var flowTemplate = template.Must(template.New("trn.flow").Parse(`<figure class="trn trn-view trn-view-flow">
<style>
.trn-view-flow{--trn-fg:#1c1b19;--trn-bg:#fffefb;--trn-line:#cdc7bb;--trn-head:#f2eee4;--trn-mark:#9a6b1f;--trn-muted:#6b655b;--trn-accent:#9a6b1f;--trn-final:#fdf4e0;box-sizing:border-box;max-width:100%;margin:0;padding:.5rem;color:var(--trn-fg);background:var(--trn-bg);font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;font-size:.95rem;line-height:1.35}
@media (prefers-color-scheme:dark){.trn-view-flow{--trn-fg:#e9e5dc;--trn-bg:#151412;--trn-line:#3b3833;--trn-head:#24211c;--trn-mark:#e0b463;--trn-muted:#a49c8e;--trn-accent:#e0b463;--trn-final:#241f14}}
.trn-flow-title{margin:0 0 .6rem;font-weight:600}
.trn-flow{list-style:none;margin:0;padding:0}
.trn-flow-step{position:relative;margin:0 0 1.15rem;padding-left:1.9rem}
.trn-flow-step:last-child{margin-bottom:0}
.trn-flow-step::before{content:"";position:absolute;left:.7rem;top:0;width:2px;height:calc(100% + 1.15rem);background:var(--trn-line)}
.trn-flow-step:first-child::before{top:1.1rem;height:calc(100% + .05rem)}
.trn-flow-step:last-child::before{height:1.1rem}
.trn-flow-step + .trn-flow-step::after{content:"";position:absolute;left:.7rem;top:-.45rem;width:0;height:0;border-left:5px solid transparent;border-right:5px solid transparent;border-top:6px solid var(--trn-line);transform:translateX(-4px)}
.trn-flow-box{position:relative;border:1px solid var(--trn-line);border-radius:.5rem;background:var(--trn-head);padding:.55rem .7rem}
.trn-flow-box::before{content:"";position:absolute;left:-1.2rem;top:1.05rem;width:1.2rem;height:2px;background:var(--trn-line)}
.trn-flow-head{margin:0;font-size:1rem;font-weight:600}
.trn-flow-num{font-variant-numeric:tabular-nums;color:var(--trn-muted);font-weight:500}
.trn-flow-chips{list-style:none;display:flex;flex-wrap:wrap;gap:.3rem;margin:.45rem 0 0;padding:0}
.trn-chip{display:inline-block;max-width:100%;border:1px solid var(--trn-line);border-radius:999px;background:var(--trn-bg);padding:.1rem .55rem;font-size:.9em;overflow-wrap:anywhere}
.trn-flow-again{margin:.35rem 0 0;color:var(--trn-muted);font-size:.9em}
.trn-flow-again-label,.trn-flow-refs-label{font-variant:small-caps;letter-spacing:.04em}
.trn-chip-again{border-style:dashed;background:transparent;color:var(--trn-muted)}
.trn-flow-refs{margin:.4rem 0 0;padding-top:.35rem;border-top:1px dashed var(--trn-line);font-size:.9em;color:var(--trn-muted)}
.trn-flow-ref{display:inline-block}
.trn-flow-ref::before{content:"← ";color:var(--trn-mark);font-weight:700}
.trn-flow-ref-num{font-variant-numeric:tabular-nums;color:var(--trn-mark);font-weight:600}
.trn-flow-ref-unknown .trn-flow-ref-num{color:var(--trn-muted);font-weight:500}
.trn-flow-step-final .trn-flow-box{border:2px solid var(--trn-accent);background:var(--trn-final)}
.trn-flow-final-badge{margin-left:.35rem;padding:0 .3rem;border:1px solid var(--trn-accent);border-radius:.25rem;color:var(--trn-accent);font-size:.7em;font-weight:600;letter-spacing:.06em;text-transform:uppercase;vertical-align:.15em}
@media print{.trn-view-flow{--trn-fg:#000;--trn-bg:#fff;--trn-line:#666;--trn-head:#fff;--trn-mark:#000;--trn-muted:#333;--trn-accent:#000;--trn-final:#fff;color:#000;background:#fff;padding:0;font-size:10pt}.trn-flow-step{break-inside:avoid;page-break-inside:avoid}.trn-flow-step-final .trn-flow-box{border-style:double;border-width:4px}}
</style>
{{if .Title}}<figcaption class="trn-title trn-flow-title">{{.Title}}</figcaption>
{{end}}<ol class="trn-flow">
{{range .Steps}}<li class="trn-flow-step{{if .Final}} trn-flow-step-final{{end}}">
<div class="trn-flow-box">
<h3 class="trn-flow-head"><span class="trn-flow-num">[{{.Number}}]</span> <span class="trn-flow-verb">{{.Verb}}</span>{{if .Final}} <span class="trn-flow-final-badge">final</span>{{end}}</h3>
{{if .Chips}}<ul class="trn-flow-chips">{{range .Chips}}<li class="trn-chip">{{.}}</li>{{end}}</ul>
{{end}}{{if .Again}}<p class="trn-flow-again"><span class="trn-flow-again-label">again</span>{{range .Again}} <span class="trn-chip trn-chip-again">{{.}}</span>{{end}}</p>
{{end}}{{if .Refs}}<p class="trn-flow-refs"><span class="trn-flow-refs-label">from</span>{{range .Refs}} <span class="trn-flow-ref{{if not .Known}} trn-flow-ref-unknown{{end}}"><span class="trn-flow-ref-num">{{if .Known}}[{{.Number}}]{{else}}#{{.Number}}{{end}}</span>{{if .Known}} <span class="trn-flow-ref-verb">{{.Verb}}</span>{{end}}</span>{{end}}</p>
{{end}}</div>
</li>
{{end}}</ol>
</figure>
`))
