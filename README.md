# Tabular Recipe Notation Renderer

The core engine of [TRN](https://github.com/aadjm/trn): a pure Go (stdlib-only)
renderer that turns the frozen TRN markdown dialect into semantic HTML —
Cooking-for-Engineers-style matrix and flow views. No network, no filesystem
access beyond the input, no state, no JavaScript in its output. The matrix view
is pure structure (no embedded CSS at all); the embedding site owns styling.

- **Contract**: [`SPEC-ENGINE.md`](https://github.com/aadjm/trn/blob/main/docs/SPEC-ENGINE.md)
  in the `aadjm/trn` repo is the source of truth for the dialect, the library API,
  and the acceptance criteria (E-A1 … E-A6).
- **Consumers**: the TRN site (`aadjm/trn`) imports this module as a library; the
  `trn-render` CLI renders files standalone.
- **Implementation**: tracked as Kanban cards `TRN-ENG-1` … `TRN-ENG-8`.
- **Constraints (non-negotiable)**: Go stdlib only; never panic on input; HTML
  generated via `html/template` escaping paths.

## Status

| Piece | State |
|---|---|
| Parser + AST (`Parse`) | implemented — `parse.go`, `types.go` |
| Warnings W1, W2, W3, W5, W6, W7 | implemented (parse time) |
| Warning W4 (ingredient never reaches the final step) | implemented (render time) — `ConvergenceWarnings`, `warnings.go` |
| Matrix view (`RenderTable`) | implemented — `render_table.go` |
| Flow view (`RenderFlow`) | implemented — `render_flow.go` |
| Presentation layer, CSS-only view switcher, standalone document (`RenderDocument`) | implemented — `presentation.go` |
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

`RenderFlow` returns the flow view (SPEC-ENGINE §4.2) as a self-contained
fragment of the same kind: one box per step in document order, numbered, with
the verb as the box header; the ingredients a step introduces as chips; the
steps a box draws on as a CSS arrow naming each source (`← [1] Crush or grind to
a powder`); and a visually distinct box for the final step. The boxes sit on a
vertical rail whose down-arrows show the sequence. The fragment is classed
(`trn-flow`, `trn-flow-step`, `trn-flow-step-final`, `trn-chip`, `trn-flow-ref`,
…), carries its own light/dark and print stylesheets, and contains no
JavaScript, no images and no external resources. `RenderFlow(nil)` returns an
empty fragment, and no input makes it panic or return an error.

Both renderers read the same AST and agree on how a reference resolves
(`firstByNumber`: the first appearance of the step number), so the views never
disagree about which step a `#n` means.

`RenderDocument` wraps both fragments in one complete page — the recipe title as
an `h1`, a CSS-only switcher between the views, and both views in the same
document. It is what the `trn-render` CLI emits for `--standalone`; the site does
not need it, because the site owns its own page and embeds the fragments. Set
`Recipe.Title` before calling to override the title shown; a recipe without a
title falls back to "Recipe". The page declares `lang="en"`, and a nil recipe
renders an empty page rather than an error.

## Presentation (SPEC-ENGINE §4.3)

The matrix view is **pure HTML** (§10: structure, not appearance): the table
carries no `<style>`, no `<script>` and no view toggle. What still ships styling
inside the engine is the flow view and the standalone page — the flow view's
boxes/connectors and the page chrome (title, switcher, panels, print rules).
Those move to the site, or are stripped, under TRN-ENG-5 REDO; the shared
baseline below is what they use until then.

- **One baseline, embedded where styling still lives.** `sharedCSS` in
  `presentation.go` holds the palette custom properties, the system font stack
  (`system-ui, -apple-system, "Segoe UI", Roboto, …` — no webfonts, no CDN), the
  `prefers-color-scheme: dark` variant and the print reset. The flow fragment and
  the standalone page embed that block verbatim, delimited by
  `/* trn:shared:start */ … /* trn:shared:end */` so a deployment's stylesheet can
  be audited, and `TestSharedStylesheetIsSingleSource` proves they carry exactly
  the same bytes — and that the matrix fragment carries none of it.
- **Zero JavaScript, CSS-only switcher.** The view switcher is a radio group —
  two `input type=radio` with `label`s, styled by `:checked`, driving the two
  panels through sibling selectors (`#trn-tab-flow:checked ~ .trn-panel-…`).
  Radios rather than a checkbox or `:target` tabs because the two views are
  mutually exclusive by nature, and a radio group already carries that meaning:
  arrow keys move between the options, assistive technology announces the
  selected one, and no state has to be hidden in a URL fragment. The inputs are
  visually hidden but stay focusable; the focus ring is drawn on the label.
- **Light and dark** come from CSS custom properties flipped by
  `prefers-color-scheme`. Every foreground/background pair the flow view uses is
  checked by `TestPaletteContrast` against the hex values actually emitted: text
  pairs clear 4.5:1 and the CSS-generated glyphs clear 3:1, in both schemes and
  in print.
- **Print prints the matrix.** The engine's page suppresses the flow panel and
  the tabs in print and forces the matrix panel visible whatever is selected on
  screen. The matrix's own page rules (landscape `@page`, fixed 8pt layout,
  repeated header row, unbreakable rows) are the site's job now that the fragment
  is pure HTML — an unstyled table still prints, it just fills the page.
- **Accessibility.** `lang` on the page, one `h1` (the recipe title) followed by
  an `h2` per view panel and the flow view's `h3` step boxes — a logical
  outline; panels are labelled by their tab (`aria-labelledby`); the matrix is a
  real `<table>` with `<th scope="col">` for step columns and `<th scope="row">`
  for ingredients, so the rowspan cells are announced with both their column and
  their row; the flow view's CSS glyphs are `aria-hidden`.
- **Re-skinning.** Every element carries a `trn-*` class (`trn-table`,
  `trn-row`, `trn-step`, `trn-ingredient`, `trn-gap`, `trn-action`, `trn-flow`,
  `trn-flow-step-final`, `trn-chip`, …), so the embedding site can restyle the
  views with its own CSS without touching the markup or the engine.

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
$ go test ./... -update  # rewrite testdata/*.table.html and *.flow.html goldens
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
the fragment and checks the box order and numbering, each box's verb, the chips
each step introduces, the "again" notes for re-listed ingredients, and one
rendered reference per written reference in order — with the boxes and refs
cross-checked against the AST for every fixture. It also covers the cases the
fixtures do not: a dangling reference (`#9`), duplicated step numbers, a step
with no items, a self reference and an empty verb, plus a no-JavaScript /
no-external-resources scan.

`golden_test.go` compares `testdata/<name>.trn.md` with both committed
`testdata/<name>.table.html` and `testdata/<name>.flow.html`, so every fixture
is rendered and compared through both views.

`presentation_test.go` covers §4.3: it locates the shared baseline in every
fragment and in the page and compares the bytes, scans the page for scripts,
event handlers and external references, checks the switcher's wiring and its
`:checked` rules (and that the inputs precede the panels, which the CSS sibling
selectors depend on), checks the heading order, `lang`, the panel labelling and
the scoped table headers, asserts the print rules, computes WCAG contrast ratios
from the palette actually emitted, and keeps one committed standalone page
(`testdata/chai.doc.html`) under golden test — the same artifact the browser
check opens. `go test ./... -update` rewrites that golden along with the two
per-view goldens.

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

`--standalone` writes `RenderDocument`: one self-contained page, both views
behind the CSS-only switcher, no request and no script. Without it the command
writes the requested view fragment(s) — the matrix, the flow view, or both
separated by a newline — which is what the site embeds.

Exit status is 0 for every document that was rendered, warnings or not; 1 if the
input could not be read or the output could not be written; 2 for a usage error
(unknown flag, bad `--view`, a second file argument). Warnings (W1 … W7, W4
included) are printed to stderr, one `trn-render: W2: …` line each, and never
change the exit status.

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
15. **E-A2's "11 ingredient rows" does not follow from §4.1.** The chai document
    has 8 unique ingredients — 11 is the item count of steps 1–4 with the three
    references counted as items. §4.1 ("rows: unique ingredients") is normative,
    so the matrix has 8 rows; `TestEA2ChaiMatrix` asserts the semantic numbers
    (8 rows × 5 columns, rowspans 4/6/8, 2 action rows, every ingredient
    covered by the finished-dish cell) instead of three invented rows. Recorded
    here rather than silently resolved.
16. **Degenerate input renders, never errors.** `RenderTable(nil)` returns an
    empty fragment; a document without steps renders the table shell (header row
    only); an empty title omits the `<caption>`; an empty verb (W7) leaves a step
    cell whose verb span is empty but which still occupies its column.
17. **Goldens are `testdata/<name>.table.html` and `testdata/<name>.flow.html`**,
    rebuilt with `go test ./... -update`. Both views are compared for every
    fixture, so neither can silently lose a fixture, and the golden covers the
    whole fragment.

The flow view's choices where §4.2 leaves room, each covered in `flow_test.go`:

18. **Reference arrows are drawn inside the referencing box, naming their
    source.** §4.2 asks for an arrow connecting the referenced step's box to the
    referencing one. A continuous connector is only straightforward between
    neighbouring boxes: the gap depends on font size, wrapping and the heights of
    the boxes in between, so an edge between non-adjacent boxes would need pixel
    (or grid-row) geometry baked into the output — and the SVG that would make it
    easy is forbidden. Instead the boxes sit on a vertical rail with a down-arrow
    into every box after the first (the sequence of steps), and each reference is
    an arrow that names the step it comes from — `from ← [1] Crush or grind to a
    powder` — in the box that makes it. The glyph is CSS-generated
    (`.trn-flow-ref::before`), so the markup carries no arrows and a re-skin can
    change them; the word "from" carries the meaning for assistive technology.
    This is the card's sanctioned fallback (borders/pseudo-elements, or a text
    arrow — never SVG/images/canvas).
19. **A chip belongs to the step that first lists the ingredient; a later
    listing becomes an "again" note.** §4.2 attaches chips to the first
    appearance, but §10 also requires ingredient strings to survive
    byte-for-byte, so a re-listed ingredient is not dropped: grocy-1 lists
    `1 tbsp cilantro` in steps 7 and 10, and step 10 renders it as a dashed,
    muted chip under an "again" label. Chips plus again notes reproduce each
    step's ingredient items exactly, in document order — the fuzz target asserts
    the counts match for arbitrary input.
20. **The final box is marked three ways**: `trn-flow-step-final` on the list
    item, a heavier accent border plus a tinted background on the box, and a
    "final" badge next to the verb — so it stays distinct in print and after
    re-skinning. It is the last step in document order, the same step W4
    measures convergence against.
21. **Chips wrap, and long ingredient text wraps inside its chip** (flex `wrap`
    plus `overflow-wrap`), so a long list or a long ingredient name never
    truncates or overflows; the card's fallback explicitly rules out truncation.

The presentation layer's choices where §4.3 leaves room, each covered in
`presentation_test.go`:

22. **The view switcher is a radio group**, not a checkbox or a `:target` tab
    pair (both of which the spec allows). Two views are mutually exclusive, and a
    radio group already means exactly that: one of two checked, arrow keys moving
    between them, the selected state announced by assistive technology, and no
    state stored in the URL or in a control that reads as "on/off". Only
    `:checked` is used as a selector, so the constraint that matters — zero
    JavaScript — holds exactly as it would for a checkbox.
23. **In the standalone page the title is shown once.** The page `h1` carries it;
    the fragments keep their own `<caption>`/`<figcaption>` so a fragment used
    alone is still self-describing, but in document context they are visually
    hidden (the caption keeps naming the table for assistive technology) rather
    than repeated above both views.
24. **Print prints the matrix, whatever is selected on screen.** The tabular view
    is the notation (§4.3, "the whole point of a notation") and the switcher is
    screen chrome, so the engine's page suppresses the flow panel and the tabs in
    print and forces the matrix panel visible. The matrix's own print styling
    (landscape `@page`, 8pt fixed layout, repeated header row, unbreakable rows)
    is **not** in the engine any more: the fragment is pure HTML, so the page
    rules that used to travel inside it now belong to the site (SPEC-SITE §7),
    which prints its own page.
25. **`lang="en"` is stated by the engine's page.** The dialect carries no
    language, the site owns its own `<html>`, and §5 defines no CLI flag for it —
    so the one document the engine writes says the language it serves instead of
    omitting the attribute (which would leave screen readers guessing at
    pronunciation). A future `--lang` flag maps onto that constant.
26. **A standalone page always carries both views; `--view` selects fragments.**
    `--standalone` is §5's "wrap in `<!doctype html>…</html>`", and the engine's
    page is the presentation layer's page: both views behind the switcher. The
    CLI therefore hands `--standalone` to `RenderDocument` unchanged and lets
    `--view` choose the fragment output only; the usage text says so rather than
    leaving a flag quietly half-applied. `--view table --standalone` is not an
    error — it is the page, whose print stylesheet already prints the matrix
    whatever is selected (decision 24). A single-view page would need another
    library API, not a second wrapper in `main.go`.
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
