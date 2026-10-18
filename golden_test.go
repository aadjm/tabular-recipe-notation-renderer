package trn

import (
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update rewrites the committed goldens instead of comparing against them:
//
//	go test ./... -update
var update = flag.Bool("update", false, "rewrite testdata/*.table.html and testdata/*.flow.html goldens")

// TestGoldenTable compares the matrix view for every fixture with its committed
// golden file (SPEC-ENGINE §6: golden-file tests, rebuild with -update).
func TestGoldenTable(t *testing.T) {
	compareGoldens(t, ".table.html", RenderTable)
}

// TestGoldenFlow does the same for the flow view. The two views share the
// fixtures and differ only in the golden suffix, so a fixture is never added
// for one view alone.
func TestGoldenFlow(t *testing.T) {
	compareGoldens(t, ".flow.html", RenderFlow)
}

// compareGoldens compares one view's rendering of every testdata/<name>.trn.md
// with the committed testdata/<name><suffix> golden.
func compareGoldens(t *testing.T, suffix string, render func(*Recipe) template.HTML) {
	t.Helper()
	for _, name := range fixtureNames(t) {
		base := strings.TrimSuffix(name, ".trn.md")
		golden := filepath.Join("testdata", base+suffix)
		t.Run(base, func(t *testing.T) {
			got := string(render(loadRecipe(t, name)))
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v (run `go test ./... -update` to create it)", golden, err)
			}
			if got != string(want) {
				t.Errorf("rendered view differs from %s%s", golden, firstDiff(string(want), got))
			}
		})
	}
	if *update {
		return
	}
	// A golden without its fixture means a stale, never-compared file.
	goldens, err := filepath.Glob("testdata/*" + suffix)
	if err != nil {
		t.Fatalf("glob goldens: %v", err)
	}
	for _, golden := range goldens {
		base := strings.TrimSuffix(filepath.Base(golden), suffix)
		if _, err := os.Stat(filepath.Join("testdata", base+".trn.md")); err != nil {
			t.Errorf("golden %s has no matching fixture", golden)
		}
	}
}

// firstDiff points at the first byte that differs, with a short window of
// context from the golden, so a golden-test failure is actionable without
// dumping the whole fragment.
func firstDiff(want, got string) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	if i == len(want) && i == len(got) {
		return ""
	}
	const window = 120
	start := i - window/2
	if start < 0 {
		start = 0
	}
	clamp := func(s string) string {
		end := start + window
		if end > len(s) {
			end = len(s)
		}
		if start > end {
			return ""
		}
		return s[start:end]
	}
	return fmt.Sprintf("\n  first difference at byte %d\n  golden: %q\n  got:    %q", i, clamp(want), clamp(got))
}
