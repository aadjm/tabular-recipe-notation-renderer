// Command trn-render renders a TRN (Tabular Recipe Notation) markdown document
// as HTML, either as the two view fragments the site embeds or as one complete
// standalone page.
//
//	trn-render [flags] [file.md | -]
//	  --view table|flow|both   which fragment(s) to write (default both)
//	  --standalone             write a complete <!doctype html>…</html> page
//	  --title <text>           override the document title
//
// A file argument of "-", or no argument at all, reads standard input. Flags
// may come before or after the file argument. Warnings (SPEC-ENGINE §3) never
// stop rendering: they are reported on stderr and the output is still written,
// so the exit status is 0 whenever the document was read and written. Only an
// I/O failure exits non-zero for content reasons; a malformed command line is a
// usage error (exit 2) and never consumes the input.
//
// The command is a thin wrapper: parsing, both views and the standalone page
// all live in the library, so the CLI and the site render identical bytes
// (SPEC-ENGINE §5). The engine emits structure only — no stylesheet, no script,
// no controls — so styling belongs to whoever embeds the output.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	trn "github.com/aadjm/tabular-recipe-notation-renderer"
)

// Exit statuses. ExitOK is returned for every document that was read and
// written, warnings or not (SPEC-ENGINE §5).
const (
	exitOK    = 0
	exitIO    = 1 // the input could not be read or the output could not be written
	exitUsage = 2 // the command line itself is wrong; nothing was read
)

// The --view values, spelled as in SPEC-ENGINE §5.
const (
	viewTable = "table"
	viewFlow  = "flow"
	viewBoth  = "both"
)

// usage is printed on --help and after a usage error.
const usage = `usage: trn-render [flags] [file.md | -]

Renders a TRN (Tabular Recipe Notation) document as HTML.

  --view table|flow|both   which view(s) to write (default both)
  --standalone             write a complete <!doctype html>...</html> page
  --title <text>           override the document title
  -h, --help               print this help

A file argument of "-" (or no argument) reads standard input. Flags may precede
or follow the file argument.

Without --standalone the output is the requested view fragment(s) — the matrix,
the flow view, or both separated by a newline. With --standalone the output is
a complete plain HTML document containing the requested view(s).

Warnings (W1-W7) are reported on stderr, one "trn-render: CODE: message" line
each, and never change the exit status.

Exit status: 0 when the document was rendered, even with warnings; 1 when the
input could not be read or the output could not be written; 2 on a usage error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options is one parsed command line.
type options struct {
	view       string // viewTable, viewFlow or viewBoth
	standalone bool   // write RenderDocument instead of the fragment(s)
	title      string // override Recipe.Title; empty means "keep the parsed title"
	file       string // input path; "" or "-" means stdin
}

// run is the whole command: parse the line, read the input, render it, write
// the output and the warnings. It never touches os.* or package-level state, so
// the tests drive it directly and a second invocation cannot see the first.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "trn-render: %v\n\n%s", err, usage)
		return exitUsage
	}

	src, err := readInput(o.file, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "trn-render: %v\n", err)
		return exitIO
	}

	recipe, warnings := trn.Parse(string(src))
	// W4 is computed at render time and comes through the same channel
	// (ConvergenceWarnings), so the CLI reports the complete W1 … W7 set.
	warnings = append(warnings, trn.ConvergenceWarnings(recipe)...)
	sortWarnings(warnings)
	if recipe != nil && o.title != "" {
		recipe.Title = o.title
	}

	if _, err := io.WriteString(stdout, render(recipe, o)); err != nil {
		fmt.Fprintf(stderr, "trn-render: writing output: %v\n", err)
		return exitIO
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "trn-render: %s: %s\n", w.Code, w.Msg)
	}
	return exitOK
}

// render produces the document bytes for o. The standalone page is the
// library's RenderDocument verbatim — the CLI adds no markup of its own, so a
// page rendered here is the one the browser verification covered. The fragment
// output is the same function pair the site calls, joined as §5 describes.
func render(r *trn.Recipe, o options) string {
	if o.standalone {
		return string(trn.RenderDocumentView(r, o.view))
	}
	return string(trn.RenderView(r, o.view))
}

// readInput reads the whole input, whether it comes from a file or stdin.
func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		src, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("reading standard input: %w", err)
		}
		return src, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return src, nil
}

// sortWarnings puts the parser's warnings and the render-time W4 warnings into
// SPEC-ENGINE §3 code order (W1 … W7), keeping each code's document order.
func sortWarnings(warnings []trn.Warning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		return warnings[i].Code < warnings[j].Code
	})
}

// errHelp is the sentinel parseArgs returns for -h/--help: not a failure, just
// "print the usage and stop".
var errHelp = errors.New("help requested")

// parseArgs parses SPEC-ENGINE §5's flag set, GNU-style: flags may appear
// before or after the file argument, values may be attached with "=" or given
// as the next argument, and "--" ends flag parsing. Exactly one positional
// argument is accepted. No other flag exists — a typo is a usage error, not a
// silently ignored word.
func parseArgs(args []string) (options, error) {
	o := options{view: viewBoth}
	var positional []string
	onlyPositional := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if onlyPositional || arg == "-" || arg == "" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		if arg == "--" {
			onlyPositional = true
			continue
		}

		name, value, attached := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "h", "help":
			if attached {
				return o, fmt.Errorf("flag %q takes no value", arg)
			}
			return o, errHelp
		case "standalone":
			if attached {
				return o, fmt.Errorf("flag %q takes no value", arg)
			}
			o.standalone = true
		case "view", "title":
			if !attached {
				if i+1 >= len(args) {
					return o, fmt.Errorf("flag --%s needs a value", name)
				}
				i++
				value = args[i]
			}
			if name == "view" {
				switch value {
				case viewTable, viewFlow, viewBoth:
					o.view = value
				default:
					return o, fmt.Errorf("invalid --view %q: want table, flow or both", value)
				}
			} else {
				if value == "" {
					return o, errors.New("--title needs a non-empty value")
				}
				o.title = value
			}
		default:
			return o, fmt.Errorf("unknown flag %q (want --view, --standalone, --title or --help)", arg)
		}
	}

	switch {
	case len(positional) > 1:
		return o, fmt.Errorf("want one input file, got %d: %s", len(positional), strings.Join(positional, " "))
	case len(positional) == 1 && positional[0] == "":
		return o, errors.New("empty input file name")
	case len(positional) == 1:
		o.file = positional[0]
	}
	return o, nil
}
