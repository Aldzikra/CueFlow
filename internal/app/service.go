// Package app wires audio -> stt -> align and publishes Wails events.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"unicode/utf16"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Aldzikra/cueflow/internal/align"
	"github.com/Aldzikra/cueflow/internal/audio"
	"github.com/Aldzikra/cueflow/internal/normalize"
	"github.com/Aldzikra/cueflow/internal/stt"
)

// ScriptWord is one displayed word: UTF-16 range in Text (JS string indices)
// and the inclusive range of aligner token indices it covers ("2026" -> 5).
type ScriptWord struct {
	S     int `json:"s"`
	E     int `json:"e"`
	First int `json:"first"`
	Last  int `json:"last"`
}

// ScriptInfo is sent once per session; the frontend never re-tokenizes.
type ScriptInfo struct {
	Text  string       `json:"text"`
	Words []ScriptWord `json:"words"`
	Lang  string       `json:"lang"`
}

type Session struct {
	Script ScriptInfo       `json:"script"`
	State  align.AlignState `json:"state"`
	Status string           `json:"status"`
}

func init() {
	application.RegisterEvent[align.AlignState]("align:state")
	application.RegisterEvent[ScriptInfo]("script:loaded")
	application.RegisterEvent[string]("session:status")
}

// Service is bound to the frontend.
type Service struct {
	app     *application.App
	overlay *application.WebviewWindow

	amu    sync.Mutex // serialises the (not thread-safe) aligner: Feed vs Seek
	mu     sync.Mutex // guards everything below
	cancel context.CancelFunc
	al     *align.Aligner
	snap   Session
}

func New() *Service { return &Service{snap: Session{Status: "idle"}} }

// Attach is called from main once the app and overlay window exist. It is a
// package function, not a method, so Wails does not bind it to the frontend.
func Attach(s *Service, app *application.App, overlay *application.WebviewWindow) {
	s.app, s.overlay = app, overlay
}

func (s *Service) ServiceShutdown() error { s.Stop(); return nil }

func (s *Service) Devices() ([]audio.Device, error) { return audio.ListDevices() }

// Snapshot lets a window that loads late (the overlay) catch up.
func (s *Service) Snapshot() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

func (s *Service) ToggleOverlay() {
	if s.overlay.IsVisible() {
		s.overlay.Hide()
	} else {
		s.overlay.Show()
	}
}

// Start begins a session. device < 0 means the system default mic.
func (s *Service) Start(script, lang string, device int) error {
	if lang != "id" && lang != "en" {
		return fmt.Errorf("unsupported language %q", lang)
	}
	key := stt.APIKey()
	if key == "" {
		return errors.New("DEEPGRAM_API_KEY not set (environment or .env)")
	}
	toks := normalize.Normalize(script, normalize.Lang(lang))
	if len(toks) == 0 {
		return errors.New("script is empty")
	}
	s.Stop()

	cfg := align.DefaultConfig()
	cfg.Lang = normalize.Lang(lang)
	al := align.New(toks, cfg)
	info := ScriptInfo{Text: script, Words: scriptWords(script, toks), Lang: lang}

	ctx, cancel := context.WithCancel(context.Background())
	pcm, err := audio.Capture(ctx, device)
	if err != nil {
		cancel()
		return err
	}

	s.mu.Lock()
	s.cancel, s.al = cancel, al
	s.snap = Session{Script: info, State: al.State(), Status: "connecting"}
	s.mu.Unlock()
	s.app.Event.Emit("script:loaded", info)
	s.app.Event.Emit("align:state", al.State())
	s.status("connecting")
	s.overlay.Show()

	words := make(chan stt.Word, 64)
	go func() {
		err := stt.Run(ctx, stt.Config{
			APIKey: key, Lang: lang,
			Logf: func(f string, a ...any) { s.status(fmt.Sprintf(f, a...)) },
		}, pcm, words)
		close(words)
		if err != nil && ctx.Err() == nil {
			s.status("error: " + err.Error())
		}
	}()
	go func() {
		for w := range words {
			s.amu.Lock()
			st := al.Feed(align.WordEvent{Word: w.Word, IsFinal: w.Final, Start: w.Start, End: w.End})
			s.amu.Unlock()
			s.publish(al, st)
		}
	}()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.status("stopped")
	}
}

// Seek moves the cursor manually (word index).
func (s *Service) Seek(cursor int) {
	s.mu.Lock()
	al := s.al
	s.mu.Unlock()
	if al == nil {
		return
	}
	s.amu.Lock()
	al.Seek(cursor)
	st := al.State()
	s.amu.Unlock()
	s.publish(al, st)
}

// publish stores and emits st.
func (s *Service) publish(al *align.Aligner, st align.AlignState) {
	s.mu.Lock()
	if s.al == al {
		s.snap.State = st
	}
	s.mu.Unlock()
	s.app.Event.Emit("align:state", st)
}

func (s *Service) status(msg string) {
	s.mu.Lock()
	s.snap.Status = msg
	s.mu.Unlock()
	s.app.Event.Emit("session:status", msg)
}

// scriptWords groups tokens sharing a source range and converts byte offsets to UTF-16.
func scriptWords(text string, toks []normalize.Token) []ScriptWord {
	u16 := make([]int, len(text)+1) // byte offset -> UTF-16 offset
	n := 0
	for i, r := range text {
		u16[i] = n
		n += len(utf16.Encode([]rune{r}))
	}
	u16[len(text)] = n
	for i := len(text) - 1; i >= 0; i-- { // continuation bytes inherit the rune start
		if u16[i] == 0 && i > 0 {
			u16[i] = u16[i-1]
		}
	}
	var ws []ScriptWord
	for i, t := range toks {
		if k := len(ws) - 1; k >= 0 && toks[i-1].Start == t.Start && toks[i-1].End == t.End {
			ws[k].Last = i
			continue
		}
		ws = append(ws, ScriptWord{u16[t.Start], u16[t.End], i, i})
	}
	return ws
}
