package app

import (
	"reflect"
	"testing"

	"github.com/Aldzikra/cueflow/internal/normalize"
)

func TestScriptWords(t *testing.T) {
	// "é" is 2 bytes / 1 UTF-16 unit; the emoji is 2 units and no token; "2026" is 5 tokens, one word.
	text := "café 😀 tahun 2026"
	got := scriptWords(text, normalize.Normalize(text, normalize.ID))
	want := []ScriptWord{{0, 4, 0, 0}, {8, 13, 1, 1}, {14, 18, 2, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
