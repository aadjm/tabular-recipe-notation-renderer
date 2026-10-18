package trn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"testing"
)

// digestLine matches the provenance block in testdata/README.md:
//
//	<sha256>  grocy-<n>.trn.md   <grocy name>
var digestLine = regexp.MustCompile(`^([0-9a-f]{64})  (grocy-[1-5]\.trn\.md)`)

// TestGrocyFixturesMatchRecordedDigests pins the five live-instance fixtures to
// the digests recorded in testdata/README.md when they were fetched from Grocy.
//
// SPEC-ENGINE §6 wants the `trn` userfield contents committed verbatim, and the
// realistic way to lose that property is not a hand edit but a tool: an editor
// or formatter that appends a final newline, or a `-update` run after someone
// "tidied" a fixture. Recipes 1–3 have no trailing newline in Grocy, so any such
// append shows up here immediately. Refetching a recipe legitimately changes the
// digest — update the README block and the goldens in the same commit.
func TestGrocyFixturesMatchRecordedDigests(t *testing.T) {
	readme, err := os.ReadFile("testdata/README.md")
	if err != nil {
		t.Fatalf("read testdata/README.md: %v", err)
	}
	want := map[string]string{}
	for _, line := range regexp.MustCompile(`\r?\n`).Split(string(readme), -1) {
		if m := digestLine.FindStringSubmatch(line); m != nil {
			want[m[2]] = m[1]
		}
	}
	if len(want) != 5 {
		t.Fatalf("testdata/README.md records %d fixture digests, want 5", len(want))
	}

	for name, digest := range want {
		src, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Errorf("read fixture %s: %v", name, err)
			continue
		}
		sum := sha256.Sum256(src)
		if got := hex.EncodeToString(sum[:]); got != digest {
			t.Errorf("%s does not match its recorded digest\n  recorded: %s\n  actual:   %s\n  "+
				"the fixture must equal the Grocy userfield byte for byte — refetch it (testdata/README.md) "+
				"or, if the recipe itself changed, update the digest and the goldens together",
				name, digest, got)
		}
	}
}

// TestGrocyFixturesKeepTrailingNewlines records the trailing-newline shape of
// the live userfields. It is redundant with the digest test by design: it says
// out loud which difference a formatter would introduce, so a failure reads as
// "an editor added a newline" rather than "some hash changed".
func TestGrocyFixturesKeepTrailingNewlines(t *testing.T) {
	for id, want := range map[int]bool{1: false, 2: false, 3: false, 4: true, 5: true} {
		name := fmt.Sprintf("testdata/grocy-%d.trn.md", id)
		src, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("read fixture %s: %v", name, err)
			continue
		}
		if got := len(src) > 0 && src[len(src)-1] == '\n'; got != want {
			t.Errorf("%s ends with a newline = %v, want %v (Grocy stores recipe %d that way)", name, got, want, id)
		}
	}
}
