# Tabular Recipe Notation Renderer

The core engine of [TRN](https://github.com/aadjm/trn): a pure Go (stdlib-only)
renderer that turns the frozen TRN markdown dialect into standalone, self-styled
HTML — Cooking-for-Engineers-style matrix and flow views. No network, no
filesystem access beyond the input, no state, no JavaScript in its output.

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
| Flow view (`RenderFlow`) | not yet implemented |
| CLI (`cmd/trn-render`) | not yet implemented |

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
```

`Parse` never returns an error and never panics: malformed documents degrade to
warnings, never to a failed parse. Warnings come back in code order (W1 … W7),
stable within a code.

`ConvergenceWarnings` is the render-time half of the warning channel: it
returns W4 for every ingredient that never reaches the final step, directly or
transitively. Callers append its result to the parser's warnings (re-sort the
merged slice if the `W1 … W7` order matters). It and the renderers share
`ancestorSteps`, the transitive back-reference closure used for matrix cells.

`RenderTable` returns the Cooking-for-Engineers matrix (SPEC-ENGINE §4.1) as a
self-contained HTML fragment: one row per unique ingredient in first-appearance
order, one column per step in document order, a cell that shows the step's verb
where the ingredient is listed, a CSS-flow-marker cell where the ingredient is
carried in through back-references, an empty cell otherwise, and a spanning
action row for every step that adds no ingredient. The fragment carries its own
classed markup (`trn-table`, `trn-cell-direct`, `trn-cell-flow`, `trn-action`, …),
its own light/dark stylesheet and a print stylesheet; it contains no JavaScript,
no images and no external resources. `RenderTable(nil)` returns an empty
fragment, and no input makes it panic or return an error.

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
$ go test ./... -update  # rewrite testdata/*.table.html goldens
$ go test -run '^$' -fuzz '^FuzzParse$' -fuzztime 30s
```

`warnings_test.go` holds one minimal, table-driven document per warning code
W1 … W7, plus the reachability cases behind W4 (transitive chains, stranded
ingredients, repeated ingredients, dangling refs, cycles, duplicated step
numbers). `parse_test.go` covers the dialect itself.

`render_test.go` reads the rendered matrix back out of the HTML and counts
(acceptance criterion E-A2): step columns, ingredient rows, action rows, flow
markers, and the convergence of every ingredient into the final column. It also
re-derives the expected cells with an independent walk for every fixture, checks
that the matrix agrees with `ConvergenceWarnings`, and pushes markup, quotes and
Unicode through every text field to prove user bytes come back intact and never
become live markup. `golden_test.go` compares `testdata/<name>.trn.md` with the
committed `testdata/<name>.table.html`; the `.table` suffix leaves
`<name>.flow.html` free for the flow view.

`testdata/chai.trn.md` and `testdata/guacamole.trn.md` are the canonical examples
from the spec; `testdata/kraft-dinner.trn.md` is the Kraft Dinner example from
[TheSonOfThomp/recipe-parser](https://github.com/TheSonOfThomp/recipe-parser)
verbatim, trailing spaces included. `testdata/grocy-1.trn.md` … `grocy-5.trn.md`
are the five real recipes carried by the `trn` userfield on the live Grocy
instance, fetched read-only — recipe text only, never the API key.

## Usage (once the CLI lands)

```console
$ trn-render recipe.trn.md --standalone > recipe.html
```

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

11. **Matrix cells** follow §4.1 literally: a cell shows the step's verb where
    the ingredient is listed in that step; it shows a flow marker where the
    ingredient is not listed but reaches the step through back-references
    (transitively); it is empty otherwise. Reachability follows references as
    written, so a forward reference (W3) also carries an ingredient into an
    earlier column; cycles terminate because each step is visited once.
12. **The flow marker is a classed, empty span.** The glyph is CSS-generated
    (`content:"→"` on `.trn-mark::before`) and hidden from assistive technology
    with `aria-hidden`, so the cell contains no text at all — the row and column
    headers already say which ingredient and step it belongs to. Any styled
    arrow/dot satisfies §4.1; this one needs no extra markup and survives
    re-skinning through the class name.
13. **A step that adds no ingredient text becomes a spanning action row** — every
    item is a reference, and a step with no items at all qualifies vacuously. The
    row sits after the ingredient rows in document order, spans every column
    (`colspan` = ingredient column + all steps) and shows `[n] verb` followed by
    the steps it references (`[n] verb`, or `#n` when the reference is dangling).
    Every step still gets a column, because §4.1 defines columns as the steps in
    document order: chai therefore has 5 columns and steps 4 and 5 as action
    rows.
14. **E-A2's "11 ingredient rows" does not follow from §4.1.** The chai document
    has 8 unique ingredients — 11 is the item count of steps 1–4 with the three
    references counted as items. §4.1 ("rows: unique ingredients") is normative,
    so the matrix has 8 rows; `TestEA2ChaiMatrix` asserts the semantic numbers
    (8 rows × 5 columns, 26 flow markers, 2 action rows, every ingredient
    converging on the final column) instead of three invented rows. Recorded
    here rather than silently resolved.
15. **Degenerate input renders, never errors.** `RenderTable(nil)` returns an
    empty fragment; a document without steps renders the table shell (header row
    only); an empty title omits the `<caption>`; an empty verb (W7) leaves a
    direct cell whose verb span is empty but whose class still marks it as
    direct.
16. **Goldens are `testdata/<name>.table.html`**, rebuilt with
    `go test ./... -update`, so the flow view can own `<name>.flow.html` over the
    same fixtures. The golden covers the whole fragment, stylesheet included.

## License

MIT
