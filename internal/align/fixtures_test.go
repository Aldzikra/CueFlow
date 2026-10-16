package align

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Aldzikra/cueflow/internal/normalize"
)

type expect struct {
	Script      string   `json:"script"`
	Lang        string   `json:"lang"`
	FinalCursor int      `json:"finalCursor"`
	Modes       []string `json:"modes"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// Every testdata/transcripts/*.json is replayed against its .expect.json.
func TestFixtures(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/transcripts/*.expect.json")
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, ef := range files {
		name := strings.TrimSuffix(filepath.Base(ef), ".expect.json")
		t.Run(name, func(t *testing.T) {
			var ex expect
			var evs []WordEvent
			readJSON(t, ef, &ex)
			readJSON(t, strings.TrimSuffix(ef, ".expect.json")+".json", &evs)
			text, err := os.ReadFile("../../testdata/" + ex.Script)
			if err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfig()
			cfg.Lang = normalize.Lang(ex.Lang)
			a := New(normalize.Normalize(string(text), cfg.Lang), cfg)

			var modes []string
			prev := a.State()
			for i, ev := range evs {
				st := a.Feed(ev)
				if st.Cursor < prev.Cursor {
					t.Fatalf("event %d (%q): cursor moved back %d -> %d", i, ev.Word, prev.Cursor, st.Cursor)
				}
				if prev.Mode == ModeImprov && st.Mode == ModeImprov && st.Cursor != prev.Cursor {
					t.Fatalf("event %d (%q): cursor moved during improv", i, ev.Word)
				}
				if st.Mode != ModeIdle && (len(modes) == 0 || modes[len(modes)-1] != string(st.Mode)) {
					modes = append(modes, string(st.Mode))
				}
				prev = st
			}
			if prev.Cursor != ex.FinalCursor {
				t.Errorf("final cursor = %d, want %d", prev.Cursor, ex.FinalCursor)
			}
			if !reflect.DeepEqual(modes, ex.Modes) {
				t.Errorf("modes = %v, want %v", modes, ex.Modes)
			}
		})
	}
}
