# Tabular Recipe Notation Renderer

A pure Go (standard-library-only) parser and HTML renderer for Tabular Recipe Notation (TRN). It turns a plain-text TRN markdown document into semantic, unstyled HTML — either a Cooking-for-Engineers-style recipe summary table (`RenderTable`), a step flow diagram (`RenderFlow`), or a complete standalone HTML page (`RenderDocument`).

## Installation

### CLI

```sh
go install github.com/aadjm/tabular-recipe-notation-renderer/cmd/trn-render@latest
```

Or build from a local checkout:

```sh
go build -o trn-render ./cmd/trn-render
```

### Go Library

```sh
go get github.com/aadjm/tabular-recipe-notation-renderer@latest
```

## Input Format

A TRN document is UTF-8 plain text:

```
document     := title? (step | continuation | blank)*
title        := first non-blank line, unless it matches /^\[\d+\]/
step         := '[' NUMBER ']' VERB ':' ITEMS
items        := ITEM (',' ITEM)*
item         := '#' NUMBER            # back-reference to a previous step
              | TEXT                  # ingredient text (preserved verbatim)
continuation := non-step line — its items are appended to the most recent step
```

### Example: Single-Line Steps (`Masala Chai`)

```markdown
Masala Chai
[1] Crush or grind to a powder: 4 green cardamoms, 2 cloves, 3g cinnamon, 2 peppercorns
[2] Boil 3 min in 360ml water until the decoction is dark: #1, 1/2 inch ginger crushed, 6g black tea
[3] Add and bring to a boil: #2, 240ml full-fat milk, 12g sugar
[4] Simmer 2 min until dark with a cream layer and no raw milk flavour: #3
[5] Pull 4-5 times from 1-1.5 ft, then strain into cups and serve: #4
```

### Example: Continuation Lines (`Guacamole`)

```markdown
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

## CLI Usage

```
trn-render [flags] [file.md | -]
  --view table|flow|both   which view(s) to write (default both)
  --standalone             write a complete <!doctype html>...</html> page
  --title <text>           override the document title
  -h, --help               print usage
```

- Pass a file path, `-`, or no positional argument to read from standard input.
- Without `--standalone`, `trn-render` outputs the requested HTML fragment(s).
- With `--standalone`, `trn-render` outputs a complete HTML document containing both views.

```sh
# Render a standalone HTML file
trn-render testdata/chai.trn.md --standalone > chai.html

# Render only the tabular recipe summary table fragment
trn-render testdata/guacamole.trn.md --view table > guacamole-table.html

# Read from stdin and override the recipe title
cat testdata/kraft-dinner.trn.md | trn-render --view flow --title "Mac & Cheese"
```

### Exit Codes

| Code | Meaning |
|---|---|
| `0` | Document rendered (warnings, if any, are printed to `stderr`) |
| `1` | Input or output I/O error |
| `2` | Command-line usage error |

## Library Usage

```go
package main

import (
	"fmt"
	"os"

	trn "github.com/aadjm/tabular-recipe-notation-renderer"
)

func main() {
	input := `Guacamole
[1] Mash: 3 ripe avocados
[2] Finely dice: 1/2 red onion, 1 jalapeno, 1 roma tomato
[3] Juice: 1 lime
[4] Fold together: #1, #2, #3, 1/4 cup chopped cilantro, 1 tsp salt`

	recipe, warnings := trn.Parse(input)
	warnings = append(warnings, trn.ConvergenceWarnings(recipe)...)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "%s: %s\n", w.Code, w.Msg)
	}

	tableHTML := trn.RenderTable(recipe) // template.HTML fragment
	flowHTML := trn.RenderFlow(recipe)   // template.HTML fragment
	pageHTML := trn.RenderDocument(recipe)

	fmt.Println(tableHTML)
	_ = flowHTML
	_ = pageHTML
}
```

### API Reference

```go
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
	Ref  int    // >0 -> step reference; 0 -> ingredient text
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

## Warning Codes

`Parse` and `ConvergenceWarnings` never fail or panic on malformed input; issues are reported as non-fatal warnings:

| Code | Condition |
|---|---|
| `W1` | Step numbers are not strictly increasing from 1 (gap, duplicate, or out of order) |
| `W2` | Reference `#n` points to a step that does not exist |
| `W3` | Forward reference (`#n` points to a step that appears later in the document) |
| `W4` | An ingredient never reaches the final step, directly or transitively |
| `W5` | Document contains no steps |
| `W6` | Document has an empty title |
| `W7` | Step has an empty verb |

## HTML Output & CSS Classes

The renderer emits semantic HTML only (no `<style>` or `<script>` tags). Style the output using the classes below:

### Table View (`RenderTable`)

- Ingredients are listed down the left column (`th.trn-ingredient`) in order of first use.
- Operations are placed left-to-right across depth columns (`td.trn-step`), with `rowspan` spanning all ingredient rows acted on directly or transitively. Parallel operations on disjoint rows share a column.
- Empty grid slots in a row are filled with `td.trn-gap` (using `colspan` where adjacent) so every row forms a complete rectangular grid.
- Preparation steps that act on no ingredients render as full-width `tr.trn-action` rows above the ingredient rows.

| Class | Element | Description |
|---|---|---|
| `trn-table` | `table` | Root table element |
| `trn-title` | `caption` | Recipe title (omitted when empty) |
| `trn-action` | `tr` | Full-width preparation row above the ingredient flow |
| `trn-action-cell` | `td` | Single `colspan` cell inside `tr.trn-action` |
| `trn-action-num`, `trn-action-verb`, `trn-action-refs`, `trn-action-ref` | `span` | Step number, verb, and referenced steps inside an action row |
| `trn-row` | `tr` | Ingredient row |
| `trn-ingredient` | `th scope="row"` | Ingredient label cell in the leftmost column |
| `trn-step` | `td` | Operation cell spanning the rows it combines (`rowspan`) |
| `trn-step-verb` | `span` | Operation verb inside `td.trn-step` |
| `trn-gap` | `td` | Empty pass-through/spacer cell keeping columns aligned (`colspan`) |

### Flow View (`RenderFlow`)

| Class | Element | Description |
|---|---|---|
| `trn-flow` | `figure` | Root flow container |
| `trn-title`, `trn-flow-title` | `figcaption` | Recipe title (omitted when empty) |
| `trn-flow-steps` | `ol` | Ordered list of step boxes |
| `trn-flow-step` | `li` | Step item (`id="trn-flow-step-<pos>"`) |
| `trn-flow-final` | `li` | Modifier class on the final step item |
| `trn-flow-box` | `div` | Step card container |
| `trn-flow-head` | `h3` | Step heading (`span.trn-flow-num` + `span.trn-flow-verb`) |
| `trn-flow-chips`, `trn-chip` | `ul`, `li` | Ingredient chips introduced at this step |
| `trn-flow-arrows`, `trn-flow-arrow` | `p`, `span` | Connectors (`data-from`, `data-to`) from referenced steps |
| `trn-flow-arrow-unknown` | `span` | Modifier on `trn-flow-arrow` when a reference is unresolved (`W2`) |

### Standalone Document (`RenderDocument`)

| Class | Element | Description |
|---|---|---|
| `trn-doc` | `body` | Document body |
| `trn-doc-title` | `h1` | Page heading |
| `trn-doc-view`, `trn-doc-view-table`, `trn-doc-view-flow` | `section` | View sections |
| `trn-doc-heading` | `h2` | Section heading (`Table` or `Flow`) |

## License

MIT
