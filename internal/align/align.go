// Package align is the voice-driven cursor engine. It is pure: tokens and
// events in, state out. No I/O, no goroutines, no audio/stt/wails imports.
package align

import (
	"github.com/Aldzikra/cueflow/internal/normalize"
)

type Mode string

const (
	ModeIdle     Mode = "idle"
	ModeTracking Mode = "tracking"
	ModeImprov   Mode = "improv"
	ModeJumped   Mode = "jumped"
)

// WordEvent is one STT word. Times are seconds. An event revises every
// pending word that starts at or after (Start - RevisionTol), so interim
// hypotheses can be re-sent and finals replace them.
type WordEvent struct {
	Word    string  `json:"word"`
	IsFinal bool    `json:"isFinal"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
}

// AlignState is the backend->frontend contract (event "align:state").
type AlignState struct {
	Cursor     int     `json:"cursor"` // index of next expected script word
	Mode       Mode    `json:"mode"`
	Confidence float64 `json:"confidence"` // 0..1, EMA of recent hit/miss
	// TentativeUntil is the end (exclusive) of the region advanced only by
	// interim words; 0 when nothing is tentative.
	TentativeUntil int            `json:"tentativeUntil,omitempty"`
	Lang           normalize.Lang `json:"lang"`
}

// Config holds every tunable. Defaults are calibrated on testdata/ fixtures.
type Config struct {
	Lang normalize.Lang

	// Window: how many script words ahead of the cursor a spoken word may
	// match (PRD: 15). Wide enough for ~2 dropped clauses, narrow enough that
	// common words rarely match by accident.
	Window int
	// WordSim: min similarity (1 - levenshtein/maxLen) for two words to match.
	// 0.8 means words <=4 letters must match exactly, 5-9 letters tolerate 1 edit.
	WordSim float64
	// MissToImprov: consecutive non-matching words before tracking -> improv.
	// 4 survives a stray misheard word or two without flipping the indicator.
	MissToImprov int
	// HitToResume: consecutive (script-order) matches needed to leave improv
	// or to accept a match further than MaxSkip. 3 words is hard to hit by chance.
	HitToResume int
	// JumpNGram: spoken words in a row that must match one script location
	// before the cursor teleports (PRD: 4).
	JumpNGram int
	// MinJumpChars: letters in that n-gram; blocks jumps on "yang di ke dan".
	MinJumpChars int
	// MaxSkip: script words that may be skipped at once in tracking mode when
	// STT drops words. Beyond it a match is only a probe (needs HitToResume).
	MaxSkip int
	// MinSkipLen: skipping (gap>0) needs a word at least this long; short
	// words are too common to justify skipping ahead on one hit.
	MinSkipLen int
	// ProbeGap: dropped script words tolerated between two probe hits.
	ProbeGap int
	// ConfAlpha: EMA weight of the newest hit/miss in Confidence.
	ConfAlpha float64
	// RevisionTol: seconds; an event revises pending words starting at or
	// after Start-RevisionTol.
	RevisionTol float64
	// BackJump: a final-backed correction may move the shown cursor back only
	// if it is at least this many words behind; smaller corrections are ignored
	// so the cursor never visibly twitches backward.
	BackJump int
}

func DefaultConfig() Config {
	return Config{
		Lang: normalize.ID, Window: 15, WordSim: 0.8, MissToImprov: 4, HitToResume: 3,
		JumpNGram: 4, MinJumpChars: 12, MaxSkip: 2, MinSkipLen: 4, ProbeGap: 1,
		ConfAlpha: 0.2, RevisionTol: 0.1, BackJump: 6,
	}
}

// core is the plain-value matcher state; copying it is a snapshot.
type core struct {
	cursor int
	mode   Mode
	miss   int
	conf   float64
	probe  int // next expected script index of the running probe
	probeN int // hits in the running probe (0 = none)
}

type pendingWord struct {
	toks    []string
	start   float64
	isFinal bool
}

type Aligner struct {
	script []string
	cfg    Config
	base   core          // state after all committed (final) words
	tail   []string      // last tokens of committed words, for n-gram jump
	pend   []pendingWord // words since base: leading finals get folded in
	shown  int           // cursor last reported; never moves back on interim noise
	state  AlignState
}

func New(script []normalize.Token, cfg Config) *Aligner {
	a := &Aligner{cfg: cfg, base: core{mode: ModeIdle}}
	for _, t := range script {
		a.script = append(a.script, t.Text)
	}
	a.state = AlignState{Mode: ModeIdle, Lang: cfg.Lang}
	return a
}

func (a *Aligner) State() AlignState { return a.state }

// Seek moves the cursor manually (hotkeys) and drops all pending evidence.
func (a *Aligner) Seek(cursor int) {
	cursor = max(0, min(cursor, len(a.script)))
	a.base = core{cursor: cursor, mode: ModeTracking, conf: a.base.conf}
	a.pend, a.tail, a.shown = nil, nil, cursor
	a.state = AlignState{Cursor: cursor, Mode: ModeTracking, Confidence: a.base.conf, Lang: a.cfg.Lang}
}

// Feed consumes one STT word event and returns the new state.
func (a *Aligner) Feed(ev WordEvent) AlignState {
	toks := tokens(ev.Word, a.cfg.Lang)
	if len(toks) == 0 {
		return a.state
	}
	// Revise: drop pending words this event supersedes, then append it.
	keep := len(a.pend)
	for keep > 0 && a.pend[keep-1].start >= ev.Start-a.cfg.RevisionTol {
		keep--
	}
	a.pend = append(a.pend[:keep], pendingWord{toks, ev.Start, ev.IsFinal})

	// Fold leading finals into the committed base.
	for len(a.pend) > 0 && a.pend[0].isFinal {
		for _, w := range a.pend[0].toks {
			a.tail = append(a.tail, w)
			a.base.step(a.script, a.tail, w, a.cfg)
		}
		if n := a.cfg.JumpNGram; len(a.tail) > n {
			a.tail = append([]string(nil), a.tail[len(a.tail)-n:]...)
		}
		a.pend = a.pend[1:]
	}

	// Replay tentative words on a copy of the base.
	c := a.base
	recent := append([]string(nil), a.tail...)
	for _, p := range a.pend {
		for _, w := range p.toks {
			recent = append(recent, w)
			c.step(a.script, recent, w, a.cfg)
		}
	}

	st := AlignState{Cursor: c.cursor, Mode: c.mode, Confidence: c.conf, Lang: a.cfg.Lang}
	if c.cursor < a.shown {
		// Interim noise never rewinds; a final-backed correction must be large.
		if len(a.pend) > 0 || a.shown-c.cursor < a.cfg.BackJump {
			st.Cursor = a.shown
		}
	}
	a.shown = st.Cursor
	if len(a.pend) > 0 && st.Cursor > a.base.cursor {
		st.TentativeUntil = st.Cursor
	}
	a.state = st
	return st
}

func tokens(word string, lang normalize.Lang) []string {
	var s []string
	for _, t := range normalize.Normalize(word, lang) {
		s = append(s, t.Text)
	}
	return s
}

// step advances the matcher by one spoken token w. recent ends with w.
func (c *core) step(script, recent []string, w string, cfg Config) {
	j := find(script, w, c.cursor, cfg.Window, cfg.WordSim)
	inReach := j >= 0 && c.mode != ModeImprov && j-c.cursor <= cfg.MaxSkip &&
		(j == c.cursor || len(w) >= cfg.MinSkipLen)

	// Global jump only on evidence of being lost, and never over a clean advance.
	if !inReach && (c.mode == ModeImprov || c.miss+1 >= cfg.MissToImprov) {
		if p, ok := jumpTarget(script, recent, c.cursor, cfg); ok {
			*c = core{cursor: p, mode: ModeJumped, conf: c.conf}
			c.score(true, cfg)
			return
		}
	}

	// Continue a running probe (consecutive script-order matches).
	if c.probeN > 0 {
		if k := find(script, w, c.probe, cfg.ProbeGap+1, cfg.WordSim); k >= 0 {
			c.probeN++
			c.probe = k + 1
			c.score(true, cfg)
			c.commitProbe(cfg)
			return
		}
		c.probeN = 0
	}

	switch {
	case inReach:
		c.cursor, c.mode, c.miss, c.probeN = j+1, ModeTracking, 0, 0
		c.score(true, cfg)
	case j >= 0: // a match, but not trustworthy alone: start a probe
		c.probe, c.probeN = j+1, 1
		if c.mode != ModeImprov {
			c.miss++
		}
		c.score(false, cfg)
		c.commitProbe(cfg)
	default:
		c.miss++
		c.score(false, cfg)
		if c.miss >= cfg.MissToImprov {
			c.mode = ModeImprov
		}
	}
}

func (c *core) commitProbe(cfg Config) {
	if c.probeN >= cfg.HitToResume {
		c.cursor, c.mode, c.miss, c.probeN = c.probe, ModeTracking, 0, 0
	}
}

func (c *core) score(hit bool, cfg Config) {
	v := 0.0
	if hit {
		v = 1
	}
	c.conf += cfg.ConfAlpha * (v - c.conf)
}

// find returns the nearest index in script[from:from+n) whose word matches w, or -1.
func find(script []string, w string, from, n int, sim float64) int {
	for i := from; i < from+n && i < len(script); i++ {
		if similar(w, script[i], sim) {
			return i
		}
	}
	return -1
}

// jumpTarget looks for the last JumpNGram spoken words anywhere in the script.
// Nearest candidate at/after the cursor wins; if all are behind, only a unique
// one is taken (ambiguous re-reads are ignored).
func jumpTarget(script, recent []string, cursor int, cfg Config) (int, bool) {
	n := cfg.JumpNGram
	if len(recent) < n || len(script) < n {
		return 0, false
	}
	gram := recent[len(recent)-n:]
	chars := 0
	for _, g := range gram {
		chars += len([]rune(g))
	}
	if chars < cfg.MinJumpChars {
		return 0, false
	}
	ahead, behind, nBehind := -1, -1, 0
	for p := 0; p+n <= len(script); p++ {
		ok := true
		for k := 0; k < n && ok; k++ {
			ok = similar(gram[k], script[p+k], cfg.WordSim)
		}
		if !ok || p+n == cursor {
			continue
		}
		if p+n > cursor {
			if ahead < 0 {
				ahead = p + n
			}
		} else {
			behind = p + n
			nBehind++
		}
	}
	switch {
	case ahead >= 0:
		return ahead, true
	case nBehind == 1:
		return behind, true
	}
	return 0, false
}

// similar: 1 - levenshtein/maxLen >= min.
func similar(a, b string, min float64) bool {
	if a == b {
		return true
	}
	ra, rb := []rune(a), []rune(b)
	m := max(len(ra), len(rb))
	if m == 0 {
		return true
	}
	return 1-float64(lev(ra, rb))/float64(m) >= min
}

func lev(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
