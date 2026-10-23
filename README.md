# Tabular Recipe Notation Renderer

The core engine of [TRN](https://github.com/aadjm/trn): a pure Go (stdlib-only)
renderer that turns the frozen TRN markdown dialect into semantic HTML —
Cooking-for-Engineers-style matrix and flow views. No network, no filesystem
access beyond the input, no state, no JavaScript in its output. Every output is
pure structure: classed HTML and nothing else, with no embedded CSS — the
embedding site owns styling (see [Styling contract](#styling-contract-spec-engine-43)).

- **Contract**: [`SPEC-ENGINE.md`](https://github.com/aadjm/trn/blob/main/docs/SPEC-ENGINE.md)
  in the `aadjm/trn` repo is the source of truth for the dialect, the library API,
  and the acceptance criteria (E-A1 … E-A6).
- **Consumers**: the TRN site (`aadjm/trn`) imports this module as a library; the
  `trn-render` CLI renders files standalone.
- **Implementation**: tracked as Kanban cards `TRN-ENG-1` … `TRN-ENG-8`.
- **Constraints (non-negotiable)**: Go stdlib only; never panic on input; HTML
  generated via `html/template` escaping paths.
- **Release**: `v0.1.0` is the first tag of the pure-HTML renderer and the version
  the TRN site pins
  (`go get github.com/aadjm/tabular-recipe-notation-renderer@v0.1.0`). The module
  has no dependencies, so it pulls nothing and needs no `go.sum` entries.

## Status

| Piece | State |
|---|---|
| Parser + AST (`Parse`) | implemented — `parse.go`, `types.go` |
| Warnings W1, W2, W3, W5, W6, W7 | implemented (parse time) |
| Warning W4 (ingredient never reaches the final step) | implemented (render time) — `ConvergenceWarnings`, `warnings.go` |
| Matrix view (`RenderTable`) | implemented — `render_table.go` |
| Flow view (`RenderFlow`) | implemented — `render_flow.go` |
| Standalone document (`RenderDocument`) | implemented — `render_document.go` (plain HTML: title + both views, no styling) |
| CLI (`cmd/trn-render`) | implemented — `cmd/trn-render/main.go` |

## The dialect

```
document     := title? (step | continuation | blank)*
title        := first non-blank line, unless it matches /^\[\d+\]/
step         := '[' NUMBER ']' VERB ':' ITEMS
items        := ITEM (',' ITEM)*
item         := '#' NUMBER            # back-reference to a step
              | TEXT                  # opaque ingredient
continuation := line without a step marker — its items are appended to the
                most recent step's item list
```

```
Masala Chai
[1] Crush or grind to a powder: 4 green cardamoms, 2 cloves, 3g cinnamon, 2 peppercorns
[2] Boil 3 min in 360ml water until the decoction is dark: #1, 1/2 inch ginger crushed, 6g black tea
[3] Add and bring to a boil: #2, 240ml full-fat milk, 12g sugar
[4] Simmer 2 min until dark with a cream layer and no raw milk flavour: #3
[5] Pull 4-5 times from 1-1.5 ft, then strain into cups and serve: #4
```

```
Guacamole
[1] Mash: 3 ripe avocados
[2] Finely dice: 1/2 red onion, 1 jalapeno, 1 roma tomato
[3] Juice: 1 lime
[4] Fold together: #1,
  #2,
  #3,
  1/4 cup chopped cilantro,
  1 tsp salt
```

Ingredient text is opaque and preserved byte-for-byte (only HTML-escaped at render
time): fractions, parentheses and any Unicode survive untouched, and only the
whitespace around an item is trimmed.

## Library API

```go
package trn

type Recipe struct {
    Title string
    Steps []Step
}

type Step struct {
    Number int
    Verb   string
    Items  []Item // order preserved
}

type Item struct {
    Ref  int    // >0 → step reference; 0 → ingredient text
    Text string
}

type Warning struct {
    Code string // W1..W7
    Msg  string
}

func Parse(input string) (*Recipe, []Warning)
func ConvergenceWarnings(r *Recipe) []Warning
func RenderTable(r *Recipe) template.HTML
func RenderFlow(r *Recipe) template.HTML
func RenderDocument(r *Recipe) template.HTML
```

`Parse` never returns an error and never panics: malformed documents degrade to
warnings, never to a failed parse. Warnings come back in code order (W1 … W7),
stable within a code.

`ConvergenceWarnings` is the render-time half of the warning channel: it
returns W4 for every ingredient that never reaches the final step, directly or
transitively. Callers append its result to the parser's warnings (re-sort the
merged slice if the `W1 … W7` order matters). It and the renderers share
`ancestorSteps`, the transitive back-reference closure used for matrix cells.

`RenderTable` returns the Cooking-for-Engineers recipe summary table
(SPEC-ENGINE §4.1) as a pure HTML fragment: one row per unique ingredient in
first-use order (`<th scope="row">`), one column per step in document order
(`<th scope="col">`, `[n]` + verb), and one cell per ingredient-bearing step
carrying the step's verb and a `rowspan` covering exactly the rows it acts on —
the ingredients it lists plus every ingredient that reaches it through
back-references. The final ingredient-bearing step therefore spans the whole
ingredient flow: that cell is the finished dish. Steps that add no ingredient
(all items are `#refs`) are full-width action rows above the ingredient flow.
Reachability is `ancestorSteps`, the same closure `ConvergenceWarnings` uses,
so the matrix and W4 cannot disagree.

The fragment is structure only — no `<style>`, no `<script>`, no view toggle
(SPEC-ENGINE §4.3/§10). It is classed (`trn-table`, `trn-row`, `trn-action`,
`trn-step`, `trn-ingredient`, `trn-gap`) so the embedding site styles it.
`RenderTable(nil)` returns an empty fragment, and no input makes it panic or
return an error.

Structural details worth knowing when reading the markup:

- A step cell starts at its first acted-on ingredient row, because that is where
  the merge begins. Rows below it are covered by the `rowspan` and carry no cell
  for that column.
- Where a later step's cell starts further down the flow, the columns before it
  still have to be occupied or the browser's implicit grid would slide the cell
  left: a run of such empty columns becomes one `<td class="trn-gap">` with a
  `colspan`. That is the `righthide` blank of the reference table, not a filler
  with content, and trailing empties emit nothing.
- A `rowspan` cannot cover a non-contiguous set, so a stranded ingredient
  sitting between two rows a step acts on is absorbed by that step's span. The
  ingredient is reported as W4 either way.

`RenderFlow` returns the flow view (SPEC-ENGINE §4.2) as a fragment of the same
kind: one box per step in document order, numbered, with the verb as the box
header (`h3`); the ingredients a step introduces as a `<ul>` of chips; one
connector element per reference, naming the box it comes from and the box it
points at; and the `trn-flow-final` class on the final step's box. The fragment
is classed (`trn-flow`, `trn-flow-step`, `trn-flow-box`, `trn-flow-final`,
`trn-chip`, `trn-flow-arrow`, …) and is pure HTML — no `<style>`, no `<script>`,
no SVG, no canvas, no image, no external resource — so the site decides how a
connector is drawn. `RenderFlow(nil)` returns an empty fragment, and no input
makes it panic or return an error.

Both renderers read the same AST and agree on how a reference resolves
(`firstByNumber`: the first appearance of the step number), so the views never
disagree about which step a `#n` means.

`RenderDocument` wraps both fragments in one complete, plain HTML page: the
recipe title as the `h1`, then the matrix and the flow view side by side in
document order, each in its own `<section>` named by an `h2` ("Table", "Flow").
It is what the `trn-render` CLI emits for `--standalone`; the site does not need
it, because the site owns its own page and embeds the fragments. There is no
switcher and no stylesheet in the page — how the two views are toggled or
skinned is the site's business (SPEC-SITE §7), and an unstyled standalone page
simply shows both. Set `Recipe.Title` before calling to override the title
shown; a recipe without a title falls back to "Recipe". The page declares
`lang="en"`, and a nil recipe renders an empty page rather than an error. Both
fragments keep their own `caption`/`figcaption`, so the title appears in the
page heading, the table caption and the flow caption: each is a label for its
own element, not decoration to be styled away.

## Styling contract (SPEC-ENGINE §4.3)

**The engine emits structure, the site emits CSS.** Every `Render*` — the two
view fragments and the standalone page — is classed semantic HTML and nothing
else: no `<style>`, no `<script>`, no `<input>` or any other control, no
`style=` attribute, no CSS custom property, no `@media`, no webfont, image,
`url()` or external reference. `document_test.go:TestRenderOutputIsPureHTML`
scans all three outputs for every fixture and for the degenerate recipes against
exactly that list, so this is enforced, not just stated.

The `trn-*` class names below are therefore the **public styling API**: a
stylesheet (the site's `static/style.css`, SPEC-SITE §7) styles the fragments by
these names alone, and renaming or dropping one breaks that stylesheet. Adding a
class is additive; changing the meaning of an existing one is a breaking change.

### Matrix view — `RenderTable` (SPEC-ENGINE §4.1)

| Class | Element | Meaning |
|---|---|---|
| `trn-table` | `table` | fragment root; a real `<table>` |
| `trn-title` | `caption` | the recipe title; omitted when the title is empty (W6) |
| `trn-head` | `tr` in `<thead>` | the header row |
| `trn-corner` | `th scope="col"` | the leading label cell ("Ingredient") |
| `trn-step` | `th` in the head, `td` in the body | a step column header, or a step's `rowspan` cell |
| `trn-step-head` | `th` (`trn-step`) | modifier marking the header variant of `trn-step` |
| `trn-step-num` | `span` | the written step number as `[n]` |
| `trn-step-verb` | `span` | the step's verb, in a header or in a `rowspan` cell |
| `trn-row` | `tr` in `<tbody>` | one ingredient row |
| `trn-ingredient` | `th scope="row"` | the ingredient label of that row |
| `trn-gap` | `td` | an empty placeholder cell (with `colspan`) holding a column open so the implicit grid cannot slide the step cell left; carries no text and no meaning |
| `trn-action` | `tr` | a full-width action row: a step that introduces no ingredient (all items are `#refs`) |
| `trn-action-cell` | `td` | that row's single `colspan` cell |
| `trn-action-num` | `span` | the step number `[n]` in an action row |
| `trn-action-verb` | `span` | the verb in an action row |
| `trn-action-refs` | `span` | wrapper around the steps the action row references |
| `trn-action-ref` | `span` | one referenced step, as `[n] verb` — or `#n` when the reference is dangling (W2) |

### Flow view — `RenderFlow` (SPEC-ENGINE §4.2)

| Class | Element | Meaning |
|---|---|---|
| `trn-flow` | `figure` | fragment root |
| `trn-title` | `figcaption` (`trn-flow-title`) | the recipe title; omitted when empty (W6) |
| `trn-flow-title` | `figcaption` | modifier with `trn-title`, marking it as the figure's title |
| `trn-flow-steps` | `ol` | the steps in document order |
| `trn-flow-step` | `li` | one step's box; `id="trn-flow-step-<pos>"` (1-based document position, the connector target) |
| `trn-flow-final` | `li` (`trn-flow-step`) | modifier on the final step in document order — exactly one per document |
| `trn-flow-box` | `div` | the box inside the list item |
| `trn-flow-head` | `h3` | the step's header, `[n]` + verb |
| `trn-flow-num` | `span` | the written step number as `[n]` |
| `trn-flow-verb` | `span` | the verb |
| `trn-flow-chips` | `ul` | the ingredients this step introduces (first appearance only); omitted when the step adds none |
| `trn-chip` | `li` | one ingredient |
| `trn-flow-arrows` | `p` | wrapper around this box's connectors; omitted when it references nothing |
| `trn-flow-arrow` | `span` | one reference: `data-to` is this box's position, `data-from` the referenced box's position (absent when the reference is dangling) |
| `trn-flow-arrow-unknown` | `span` (`trn-flow-arrow`) | modifier: the reference names a step that does not exist (W2) |
| `trn-flow-arrow-num` | `span` | the source step's `[n]`, as text |
| `trn-flow-arrow-verb` | `span` | the source step's verb, as text |

A connector carries `data-from`/`data-to` and readable text (`[n]` + the source
verb), so a stylesheet may draw the line between the two boxes by id, and an
unstyled page — or a screen reader — still states the relationship.

### Standalone document — `RenderDocument` (SPEC-ENGINE §5 `--standalone`)

| Class | Element | Meaning |
|---|---|---|
| `trn-doc` | `body` | the page root |
| `trn-doc-title` | `h1` | the page heading: the recipe title (or "Recipe", W6) |
| `trn-doc-view` | `section` | one view, named by its `h2` |
| `trn-doc-view-table` | `section` (`trn-doc-view`) | the matrix section, first |
| `trn-doc-view-flow` | `section` (`trn-doc-view`) | the flow section, second |
| `trn-doc-heading` | `h2` | the section's name: "Table" or "Flow" |

### Retired names

The pure-HTML rewrite removed the presentation layer, so these are **not**
emitted any more and a stylesheet must not expect them: `trn-view`,
`trn-view-flow`, `trn-view-toggle`, `trn-views`, `trn-tab`, `trn-tab-table`,
`trn-tab-flow`, `trn-panel`, `trn-panel-table`, `trn-panel-flow`,
`trn-panel-heading`, `trn-label-table`, `trn-label-flow`, `trn-sr-only`,
`trn-flow-ref`, `trn-flow-refs`, `trn-flow-ref-num`, `trn-flow-ref-verb`,
`trn-flow-ref-unknown`, `trn-flow-step-final`, `trn-flow-final-badge`,
`trn-chip-again`, `trn-flow-again`, `trn-flow-again-label`.

### What the engine deliberately does not do

- **No view toggle.** Two views are two sections in the standalone page; the
  site decides how to switch between the fragments it embeds (CSS-only, per
  SPEC-SITE §7).
- **No dark mode, no palette, no fonts.** `prefers-color-scheme`, print rules
  and the type stack all live in the site's stylesheet.
- **No print stylesheet.** An unstyled table still prints; the site's page
  rules make the matrix print well.
- **No geometry.** Connectors carry ids in `data-from`/`data-to`, never
  coordinates, SVG or canvas: how far apart two boxes sit depends on the
  stylesheet, so drawing is the site's job.


## Warning codes

| Code | Condition | When |
|---|---|---|
| W1 | Step numbers not strictly increasing from 1 (gap, duplicate, out of order) | parse |
| W2 | Reference `#n` points to a step that does not exist | parse |
| W3 | Forward reference (references a step that appears later) | parse |
| W4 | An ingredient never reaches the final step, directly or transitively | render |
| W5 | Document contains no steps | parse |
| W6 | Empty title | parse |
| W7 | Step verb is empty | parse |

## Tests

```console
$ go test ./...          # fixtures, edge cases, invariants, goldens
$ go test -race ./...
$ go test ./... -update  # rewrite testdata/*.table.html, *.flow.html and *.doc.html goldens
$ go test -run '^$' -fuzz '^FuzzParse$' -fuzztime 30s
```

`warnings_test.go` holds one minimal, table-driven document per warning code
W1 … W7, plus the reachability cases behind W4 (transitive chains, stranded
ingredients, repeated ingredients, dangling refs, cycles, duplicated step
numbers). `parse_test.go` covers the dialect itself.

`render_test.go` reads the rendered matrix back out of the HTML by rebuilding the
table grid exactly as a browser lays it out — rowspans, colspans and all — and
counts (acceptance criterion E-A2): step columns, ingredient rows, action rows,
each step's `rowspan`, and the convergence of every ingredient into the
finished-dish cell. It re-derives every span with an independent reachability
walk for every fixture, checks that the matrix agrees with `ConvergenceWarnings`,
fails if any cell lands in the wrong grid column (the gap-cell contract), proves
the fragment carries no styling or script, and pushes markup, quotes and Unicode
through every text field to prove user bytes come back intact and never become
live markup.

`flow_test.go` does the same for the flow view: it reads the boxes back out of
the fragment and checks the box order, positions and numbering, each box's verb,
the chips each step introduces (first appearance only), the class on the final
box, and one connector per written reference in order — with boxes, chips and
connectors cross-checked against the AST for every fixture, including that every
connector's `data-from`/`data-to` names a box that is really in the fragment. It
also covers the cases the fixtures do not: a dangling reference (`#9`),
duplicated step numbers, a step with no items, a self reference and an empty
verb, plus the pure-HTML scan (no `<style>`, `<script>`, `<svg>`, `<canvas>`,
`<img>`, no external resource) that is acceptance criterion 2.

`golden_test.go` compares `testdata/<name>.trn.md` with both committed
`testdata/<name>.table.html` and `testdata/<name>.flow.html`, so every fixture
is rendered and compared through both views.

`document_test.go` covers §4.3 on the standalone page and the pure-HTML contract
across the engine: `TestRenderOutputIsPureHTML` scans all three outputs for every
fixture — and for the nil and empty recipes — against the banned-token list
(`<style>`, `<script>`, `<input>`, `style=`, `--trn-`, `@media`, `url()`, `src=`,
`href=`, event handlers, absolute URLs, SVG/canvas/image/iframe), so no code path
can smuggle presentation or external references into the output. Around that it
checks the page's `doctype`, `lang`, `<title>`, one `h1`, two `h2` sections, the
heading order, both views present in order with their scoped table headers, the
empty-title fallback (W6), escaping of every text field, and the nil-recipe page.
It also keeps one committed standalone page (`testdata/chai.doc.html`) under
golden test — the same artifact a browser opens. `go test ./... -update` rewrites
that golden along with the two per-view goldens.

`testdata/chai.trn.md` and `testdata/guacamole.trn.md` are the canonical examples
from the spec; `testdata/kraft-dinner.trn.md` is the Kraft Dinner example from
[TheSonOfThomp/recipe-parser](https://github.com/TheSonOfThomp/recipe-parser)
verbatim, trailing spaces included. `testdata/grocy-1.trn.md` … `grocy-5.trn.md`
are the five real recipes carried by the `trn` userfield on the live Grocy
instance, fetched read-only — recipe text only, never the API key. They are
committed byte for byte, so recipes 1–3 keep the userfield's missing final
newline; a test pins each file to a digest recorded at fetch time, and
`testdata/README.md` documents the provenance and the refetch procedure.

`cmd/trn-render` tests the wrapper, not the renderers: the fragment output is
compared with `RenderTable` / `RenderFlow` (and with the committed per-view
goldens), the standalone page with the committed `testdata/chai.doc.html`, so the
CLI cannot drift from the library. Around that it pins the command-line contract
itself — flags before and after the file argument, both stdin spellings,
`--title` escaping, warnings on stderr with exit 0, I/O failures with exit 1,
usage errors with exit 2, `--`, and two consecutive runs that must not share
state.

## Usage

```
trn-render [flags] [file.md | -]
  --view table|flow|both   which view(s) to write (default both)
  --standalone             write a complete <!doctype html>…</html> page
  --title <text>           override the document title
  -h, --help               print the usage
```

`file.md`, `-`, or no argument at all reads standard input. Flags may come
before or after the file argument, values may be attached with `=` or given as
the next argument, and `--` ends flag parsing.

```console
$ go build ./cmd/trn-render                 # or: go install …/cmd/trn-render@latest
$ trn-render recipe.trn.md --standalone > recipe.html
$ trn-render recipe.trn.md --view table > matrix.html
$ cat recipe.trn.md | trn-render
$ trn-render - < recipe.trn.md
```

`--standalone` writes `RenderDocument`: one plain HTML page, the title and both
views in document order, no stylesheet, no script and no request. Without it the
command writes the requested view fragment(s) — the matrix, the flow view, or
both separated by a newline — which is what the site embeds.

Exit status is 0 for every document that was rendered, warnings or not; 1 if the
input could not be read or the output could not be written; 2 for a usage error
(unknown flag, bad `--view`, a second file argument). Warnings (W1 … W7, W4
included) are printed to stderr, one `trn-render: W2: …` line each, and never
change the exit status.

### As a library (what the TRN site does)

```go
import (
    "html/template"

    trn "github.com/aadjm/tabular-recipe-notation-renderer"
)

recipe, warnings := trn.Parse(userfieldText)        // never an error, never a panic
warnings = append(warnings, trn.ConvergenceWarnings(recipe)...) // adds the W4 half
table := trn.RenderTable(recipe)                    // template.HTML fragment
flow := trn.RenderFlow(recipe)                      // template.HTML fragment
page := trn.RenderDocument(recipe)                  // complete page (CLI --standalone)
```

```console
$ go get github.com/aadjm/tabular-recipe-notation-renderer@v0.1.0
```

Both fragments are `template.HTML`, so a handler writes them into its own page
with `{{.Table}}` / `{{.Flow}}` and styles them by the `trn-*` classes above. The
CLI is a thin wrapper over the same calls, so a page the site builds and a page
`trn-render --standalone` writes carry identical markup.

SPEC-ENGINE §5 writes the path as `engine/cmd/trn-render/main.go`; this
repository's root **is** the engine module, so the command lives at
`cmd/trn-render/main.go`.

## Decisions

Where the written rules leave room, the parser does the following. Each choice is
covered by a test in `parse_test.go`.

1. **Indented step lines are steps.** Lines are left-trimmed before the step
   pattern is applied, because the reference implementation indents its steps
   under the title (the Kraft Dinner fixture). `  [1] Boil: …` is a step;
   `  #2,` is a continuation.
2. **Any non-blank, non-step line after the title continues the previous step** —
   whether or not it is indented. With no preceding step there is nothing to
   append to, so the text is dropped silently; the dialect has no warning code
   for stray prose.
3. **Empty items are dropped.** Trailing or doubled commas therefore never
   produce empty ingredients (`Fold together: #1,` followed by a continuation
   `#2,` yields `#1, #2`).
4. **`#0` and over-long digit runs are ingredient text, not references.**
   `Item.Ref == 0` is the ingredient sentinel, so `#0` cannot be a reference; it
   is preserved verbatim. The same applies to numbers that do not fit in an
   `int`.
5. **A step number too long for an `int` degrades to `Number: 0`** plus one W1 —
   the step itself is still parsed, never dropped and never a panic.
6. **`Item.Text` is empty for references**; the value lives in `Ref` (the type
   documents `Text` as ingredient text).
7. **W1 fires per out-of-sequence position.** `[1] [2] [2]` reports one W1 (the
   third step, which would have to be 3); `[3] [2] [1]` reports two (positions 1
   and 3 — position 2 happens to hold the right number). A duplicated number
   resolves to its first appearance when a reference is resolved.
8. **W4 is a render-time warning.** Reachability needs the whole reference
   graph, so `Parse` reserves the code and never emits it; `ConvergenceWarnings`
   reports it through the same `[]Warning` channel. The final step is the last
   step in document order, ingredient identity is the exact item text, and a
   reference resolves to the first appearance of its step number — the same
   rule used for W2/W3. Dangling references and reference cycles are handled
   without error: cycles terminate, dangling refs are ignored (already W2).
9. **The title is trimmed** of surrounding whitespace, and a leading UTF-8 BOM is
   stripped (invisible in editors, otherwise absorbed into the title).
10. **Only ASCII trailing whitespace is ignored** per line (space, tab, `\r`,
    `\v`, `\f`), so CRLF input parses identically to LF. Nothing inside an item
    is normalised.

The renderer's choices where §4 leaves room, each covered in `render_test.go`:

11. **Matrix cells follow §4.1 literally: one cell per ingredient-bearing step,
    spanning the rows it acts on.** The cell shows the step's verb and nothing
    else — no per-cell verbs where an ingredient is merely carried in, no flow
    markers, no empty cells. Reachability follows references as written, so a
    forward reference (W3) also carries an ingredient into an earlier column;
    cycles terminate because each step is visited once.
12. **A cell starts at its first acted-on row and runs to its last.** That is
    where the C4E merge begins (the cell that starts the mixing stage spans the
    ingredients it takes), and it is the only reading under which an HTML
    `rowspan` can express "the rows this step acts on". A non-contiguous acted
    set — a stranded ingredient between two rows the step does act on — is
    absorbed by the span, because a `rowspan` cannot skip a row; the ingredient
    is already reported as W4.
13. **Gap cells keep the grid rectangular.** When a step's cell starts below the
    first ingredient row, the columns to its left in that row are occupied by a
    classed, empty `trn-gap` cell with a `colspan` — the `righthide` blank of the
    reference table. Without it the browser's implicit grid slides the step cell
    left into the wrong column. They carry no text and no meaning; trailing
    empties in a row are omitted.
14. **A step that adds no ingredient text becomes a full-width action row** —
    every item is a reference, and a step with no items at all qualifies
    vacuously. The row spans every column (`colspan` = ingredient column + all
    steps) and shows `[n] verb` followed by the steps it references (`[n] verb`,
    or `#n` when the reference is dangling). Per §4.1 and the Banana Bread
    template these rows sit **above** the ingredient flow, in document order.
    Every step still gets a column, because §4.1 defines columns as the steps in
    document order: chai therefore has 5 columns and steps 4 and 5 as action
    rows.
15. **E-A2 counts 8 ingredient rows, and the spec now says so.** An earlier revision
    of E-A2 said "11 ingredient rows" — 11 is the item count of steps 1–4 with the
    three references counted as items, so it contradicted §4.1 ("rows: unique
    ingredients"), which is normative. The criterion was corrected to 8 in the spec
    (`aadjm/trn` 44fac65); `TestEA2ChaiMatrix` asserts the semantic numbers
    (8 rows × 5 columns, rowspans 4/6/8, 2 action rows, every ingredient covered by
    the finished-dish cell), so the implemented matrix and E-A2 agree with no
    invented rows.
16. **Degenerate input renders, never errors.** `RenderTable(nil)` returns an
    empty fragment; a document without steps renders the table shell (header row
    only); an empty title omits the `<caption>`; an empty verb (W7) leaves a step
    cell whose verb span is empty but which still occupies its column.
17. **Goldens are `testdata/<name>.table.html` and `testdata/<name>.flow.html`**,
    rebuilt with `go test ./... -update`. Both views are compared for every
    fixture, so neither can silently lose a fixture, and the golden covers the
    whole fragment.

The flow view's choices where §4.2 leaves room, each covered in `flow_test.go`:

18. **A connector is one classed element naming both ends.** §4.2 asks for an
    arrow "from each referenced step's box to the referencing step's box",
    emitted as a classed connector element the site styles. The line itself is
    left to the site: how far apart two boxes sit depends on font size, wrapping
    and the heights of everything between them, so baking geometry (or the SVG
    that would make it easy) into the engine is exactly what the card forbids.
    Each reference therefore renders as
    `<span class="trn-flow-arrow" data-from="1" data-to="4">` inside the box that
    writes it, with `data-from`/`data-to` holding the 1-based document positions
    of the two boxes — the same numbers as their `id="trn-flow-step-N"`. The
    element also carries the source's written number and verb as text, so the
    relationship survives with no CSS at all and assistive technology reads it.
    A reference to a step that does not exist (W2) renders as
    `trn-flow-arrow trn-flow-arrow-unknown` with `#n` and no `data-from`: there
    is no box to point from.
19. **Box ids use document position, not the written step number.** Gaps and
    duplicated numbers are warnings (W1), not errors, so the written number is
    not a unique key — two boxes can both say `[1]`. Positions are unique, so
    every connector target exists and every arrow resolves to exactly one box
    (`TestFlowConnectorsPointAtRenderedBoxes`). The box header still shows the
    document's number, never a renumbered one.
20. **A chip belongs to the step that first lists the ingredient, and nowhere
    else.** §4.2 attaches chips to the first appearance only, so a re-listing is
    not a second chip: grocy-1 lists `1 tbsp cilantro` in steps 7 and 10, and
    step 10 carries no chip for it. The fuzz target asserts the chip count equals
    the number of distinct ingredient texts for arbitrary input, and
    `TestFlowChipsFirstAppearanceOnly` asserts it per fixture.
21. **The final box is marked by one class.** `trn-flow-final` on the final step's
    box (`li.trn-flow-step`) — exactly one per document, on the last step in
    document order, the same step W4 measures convergence against. How it looks
    (border, tint, badge) is the site's; the engine contributes the hook and
    nothing else. Chip wrapping and long ingredient names are the site's too: the
    markup never truncates text, so a `<ul>` of chips can only wrap.

The document wrapper's choices where §4.3/§5 leave room, each covered in
`document_test.go`:

22. **The standalone page is a plain document.** §4.3 puts presentation in the
    embedding site, so `RenderDocument` emits a doctype, `lang`, `<title>`, an
    `h1` and one `<section>` per view named by its own `h2` — and nothing else:
    no stylesheet, no switcher, no print rules. Both views are simply present in
    order; whether they become tabs, a toggle or two pages is the site's
    decision (SPEC-SITE §7). `TestRenderOutputIsPureHTML` scans the page, not
    just the fragments, for `<style>`, `<script>`, `<input>`, `style=`,
    `--trn-`, `@media` and every external reference, so the presentation layer
    cannot creep back in.
23. **The title is labelled at each level, and the duplicates stay.** The page
    `h1` names the page, the table's `<caption>` names the table, the figure's
    `<figcaption>` names the figure. Without CSS there is no way to hide the
    inner labels, and hiding them would be presentation anyway: each is the
    accessible name of its own element, and a fragment used alone still
    describes itself. A site embedding the fragments hides its own copies.
24. **Heading order is h1 → h2 per view → h3 flow boxes.** The sections carry no
    `aria-labelledby` wiring any more because they need none: a `<section>` is
    named by the heading inside it, so the `h2` that shows "Table" or "Flow" is
    also its accessible name. The matrix stays a real `<table>` with
    `<th scope="col">` for step columns and `<th scope="row">` for ingredients,
    so a `rowspan` cell is announced with both its column and its row, and the
    flow connectors stay text (`[n]` + the source verb), not just drawn.
25. **`lang="en"` is stated by the engine's page.** The dialect carries no
    language, the site owns its own `<html>`, and §5 defines no CLI flag for it —
    so the one document the engine writes says the language it serves instead of
    omitting the attribute (which would leave screen readers guessing at
    pronunciation). A future `--lang` flag maps onto that constant.
26. **A standalone page always carries both views; `--view` selects fragments.**
    `--standalone` is §5's "wrap in `<!doctype html>…</html>`", and both views in
    one file is what that page is for — there is no switcher, so it is simply the
    matrix followed by the flow view. The CLI therefore hands `--standalone` to
    `RenderDocument` unchanged and lets `--view` choose the fragment output only;
    the usage text says so rather than leaving a flag quietly half-applied.
    `--view table --standalone` is not an error, it is the page. A single-view
    page would need another library API, not a second wrapper in `main.go`.
27. **The CLI parses its own command line, GNU-style.** `flag` stops at the
    first positional argument, so `trn-render fixture.md --view table
    --standalone` — the order the acceptance criteria use — would lose its
    flags. The wrapper accepts them before or after the file, `--flag=value` and
    `--flag value`, and `--` to end flags; `-` stays a positional and means
    stdin. It is a ~60-line loop, still stdlib only, and it validates `--view`
    instead of silently ignoring a typo.
28. **Exit 2 is for the command line, not the document.** §5 says non-zero exit
    only on I/O failures, which is exactly true of the document path: any input
    that can be read renders and exits 0, warnings or not. A malformed command
    line has no document to render and also matches the convention Go's own
    `flag` package enforces (it exits 2 on an unknown flag), so usage errors
    exit 2 with the reason and the usage on stderr. The two failure classes are
    documented apart so a script can tell "bad invocation" from "bad path".
29. **The CLI reports the whole W1 … W7 set, in code order.** `Parse` returns
    W1, W2, W3, W5, W6 and W7; W4 only exists at render time
    (`ConvergenceWarnings`), so the wrapper merges the two slices and
    stable-sorts by code — the same order §3 promises for the parser's half —
    before printing them as `trn-render: W4: …` lines on stderr. §5 says
    warnings are printed when standalone; the wrapper prints them in fragment
    mode too, because stdout is the artifact in both modes and stderr is free.
    Warnings never suppress output: the document renders byte for byte as the
    library would render it.

## License

MIT
