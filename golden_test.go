package trn

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update rewrites the committed goldens instead of comparing against them:
//
//	go test ./... -update
var update = flag.Bool("update", false, "rewrite testdata/*.table.html goldens")

// TestGoldenTable compares the matrix view for every fixture with its committed
// golden file (SPEC-ENGINE §6: golden-file tests, rebuild with -update).
//
// The naming leaves room for the flow view: `<name>.table.html` here, and
// `<name>.flow.html` for RenderFlow, so both views share the same fixtures
// without overwriting each other's goldens.
func TestGoldenTable(t *testing.T) {
	fixtures := fixtureNames(t)
	for _, name := range fixtures {
		base := strings.TrimSuffix(name, ".trn.md")
		golden := filepath.Join("testdata", base+".table.html")
		t.Run(base, func(t *testing.T) {
			r := loadRecipe(t, name)
			got := string(RenderTable(r))
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
				t.Errorf("rendered matrix differs from %s%s", golden, firstDiff(string(want), got))
			}
		})
	}
	if !*update {
		// A golden without its fixture means a stale, never-compared file.
		goldens, err := filepath.Glob("testdata/*.table.html")
		if err != nil {
			t.Fatalf("glob goldens: %v", err)
		}
		for _, golden := range goldens {
			base := strings.TrimSuffix(filepath.Base(golden), ".table.html")
			if _, err := os.Stat(filepath.Join("testdata", base+".trn.md")); err != nil {
				t.Errorf("golden %s has no matching fixture", golden)
			}
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
