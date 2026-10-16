package align

import (
	"strings"
	"testing"

	"github.com/Aldzikra/cueflow/internal/normalize"
)

func newEN(script string) *Aligner {
	cfg := DefaultConfig()
	cfg.Lang = normalize.EN
	return New(normalize.Normalize(script, cfg.Lang), cfg)
}

// say feeds words as final events with increasing time.
func say(a *Aligner, t *float64, words string) AlignState {
	var st AlignState
	for _, w := range strings.Fields(words) {
		*t += 0.3
		st = a.Feed(WordEvent{Word: w, IsFinal: true, Start: *t, End: *t + 0.2})
	}
	return st
}

const script = "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec romeo sierra tango uniform victor whiskey xray yankee zulu ant bee cat dog"

func TestAlign(t *testing.T) {
	tests := []struct {
		name       string
		spoken     string
		wantCursor int
		wantMode   Mode
	}{
		{"straight", "alpha bravo charlie", 3, ModeTracking},
		{"dropped word is skipped", "alpha charlie delta", 4, ModeTracking},
		{"fuzzy match", "alpha bravo charlee", 3, ModeTracking},
		{"short common word does not skip ahead", "alpha xxx xxx hotel", 1, ModeTracking},
		{"far single hit does not move", "alpha india", 1, ModeTracking},
		{"far run of 3 moves (probe)", "alpha xx xx xx xx hotel india juliet", 10, ModeTracking},
		{"improv holds cursor", "alpha bravo xx yy zz ww", 2, ModeImprov},
		{"improv then resume", "alpha bravo xx yy zz ww charlie delta echo", 5, ModeTracking},
		{"n-gram jump", "alpha bravo uniform victor whiskey xray", 24, ModeJumped},
		{"jump needs enough letters", "alpha bravo xx yy zz ww a b c d", 2, ModeImprov},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, now := newEN(script), 0.0
			st := say(a, &now, tc.spoken)
			if st.Cursor != tc.wantCursor || st.Mode != tc.wantMode {
				t.Errorf("got cursor=%d mode=%s, want %d %s", st.Cursor, st.Mode, tc.wantCursor, tc.wantMode)
			}
		})
	}
}

func TestAmbiguousJumpPrefersAhead(t *testing.T) {
	a, now := newEN("one two three four five six seven eight nine ten one two three four"), 0.0
	// 4-gram "one two three four" appears at 0 and 10; cursor at 0 -> nearest ahead of cursor 4.
	a.Seek(5)
	say(a, &now, "zz yy xx")
	st := say(a, &now, "one two three four")
	if st.Cursor != 14 {
		t.Errorf("cursor = %d, want 14", st.Cursor)
	}
}

func TestSeek(t *testing.T) {
	a, now := newEN(script), 0.0
	say(a, &now, "alpha bravo charlie delta")
	a.Seek(1)
	if st := a.State(); st.Cursor != 1 || st.Mode != ModeTracking {
		t.Fatalf("after Seek: %+v", st)
	}
	if st := say(a, &now, "bravo"); st.Cursor != 2 {
		t.Errorf("cursor = %d, want 2", st.Cursor)
	}
}

func TestInterim(t *testing.T) {
	a := newEN(script)
	feed := func(w string, final bool, start float64) AlignState {
		return a.Feed(WordEvent{Word: w, IsFinal: final, Start: start, End: start + 0.2})
	}
	feed("alpha", false, 1)
	st := feed("bravo", false, 1.3)
	if st.Cursor != 2 || st.TentativeUntil != 2 {
		t.Fatalf("interim: %+v, want cursor 2 tentative 2", st)
	}
	// bravo revised to something else: cursor must not visibly go back.
	st = feed("zzz", false, 1.3)
	if st.Cursor != 2 {
		t.Errorf("interim revision rewound cursor to %d", st.Cursor)
	}
	// finals confirm; nothing tentative left.
	feed("alpha", true, 1)
	st = feed("bravo", true, 1.3)
	if st.Cursor != 2 || st.TentativeUntil != 0 {
		t.Errorf("after finals: %+v, want cursor 2 tentative 0", st)
	}
}

func TestFinalCorrectionBackward(t *testing.T) {
	a := newEN(script)
	// interim races ahead on 3 far-ahead script words (probe commit)...
	for i, w := range []string{"hotel", "india", "juliet"} {
		a.Feed(WordEvent{Word: w, Start: float64(i) + 1})
	}
	if a.State().Cursor != 10 {
		t.Fatalf("setup cursor = %d", a.State().Cursor)
	}
	// ...finals say it was actually the start of the script: >= BackJump behind, accepted.
	for i, w := range []string{"alpha", "bravo", "charlie"} {
		a.Feed(WordEvent{Word: w, IsFinal: true, Start: float64(i) + 1})
	}
	if c := a.State().Cursor; c != 3 {
		t.Errorf("cursor = %d, want 3", c)
	}
}
