// Command cueflow-cli is a dev tool: it feeds the aligner either from a
// transcript fixture (--transcript) or the live mic via Deepgram (--live) and
// prints the cursor timeline.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Aldzikra/cueflow/internal/align"
	"github.com/Aldzikra/cueflow/internal/audio"
	"github.com/Aldzikra/cueflow/internal/normalize"
	"github.com/Aldzikra/cueflow/internal/stt"
)

func main() {
	scriptPath := flag.String("script", "", "script text file")
	transPath := flag.String("transcript", "", "transcript JSON ([]WordEvent) to replay")
	live := flag.Bool("live", false, "use the microphone + Deepgram (needs DEEPGRAM_API_KEY)")
	device := flag.Int("device", -1, "capture device index for --live (default: system default)")
	list := flag.Bool("list-devices", false, "list capture devices and exit")
	lang := flag.String("lang", "id", "id|en")
	flag.Parse()

	var err error
	switch {
	case *list:
		err = listDevices()
	case *scriptPath == "" || (*transPath == "") == !*live || (*lang != "id" && *lang != "en"):
		flag.Usage()
		os.Exit(2)
	case *live:
		err = runLive(*scriptPath, normalize.Lang(*lang), *device)
	default:
		err = runReplay(*scriptPath, *transPath, normalize.Lang(*lang))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func listDevices() error {
	devs, err := audio.ListDevices()
	if err != nil {
		return err
	}
	for _, d := range devs {
		def := ""
		if d.IsDefault {
			def = " (default)"
		}
		fmt.Printf("%d\t%s%s\n", d.Index, d.Name, def)
	}
	return nil
}

func newAligner(scriptPath string, lang normalize.Lang) (*align.Aligner, int, error) {
	text, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, 0, err
	}
	cfg := align.DefaultConfig()
	cfg.Lang = lang
	toks := normalize.Normalize(string(text), lang)
	return align.New(toks, cfg), len(toks), nil
}

func printState(ev align.WordEvent, st align.AlignState, n int, extra string) {
	kind, tent := "F", ""
	if !ev.IsFinal {
		kind = "i"
	}
	if st.TentativeUntil > 0 {
		tent = fmt.Sprintf(" tentative<%d", st.TentativeUntil)
	}
	fmt.Printf("%7.2fs %s %-14s cursor=%3d/%d mode=%-8s conf=%.2f%s%s\n",
		ev.Start, kind, ev.Word, st.Cursor, n, st.Mode, st.Confidence, tent, extra)
}

func runReplay(scriptPath, transPath string, lang normalize.Lang) error {
	a, n, err := newAligner(scriptPath, lang)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(transPath)
	if err != nil {
		return err
	}
	var evs []align.WordEvent
	if err := json.Unmarshal(raw, &evs); err != nil {
		return fmt.Errorf("parse %s: %w", transPath, err)
	}
	for _, ev := range evs {
		printState(ev, a.Feed(ev), n, "")
	}
	return nil
}

func runLive(scriptPath string, lang normalize.Lang, device int) error {
	key := apiKey()
	if key == "" {
		return fmt.Errorf("DEEPGRAM_API_KEY not set (env or .env)")
	}
	a, n, err := newAligner(scriptPath, lang)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pcm, err := audio.Capture(ctx, device)
	if err != nil {
		return err
	}
	t0 := time.Now()
	words := make(chan stt.Word, 64)
	errc := make(chan error, 1)
	go func() {
		errc <- stt.Run(ctx, stt.Config{
			APIKey: key, Lang: string(lang),
			Logf: func(f string, args ...any) { fmt.Fprintf(os.Stderr, f+"\n", args...) },
		}, pcm, words)
		close(words)
	}()
	fmt.Fprintln(os.Stderr, "listening... Ctrl-C to stop")

	prev := a.State().Cursor
	hint := time.NewTimer(5 * time.Second)
	defer hint.Stop()
	for w := range recvWords(words, hint.C) {
		ev := align.WordEvent{Word: w.Word, IsFinal: w.Final, Start: w.Start, End: w.End}
		st := a.Feed(ev)
		extra := ""
		if st.Cursor > prev {
			// Latency: wall time since capture start minus when the word ended.
			// Approximate: assumes the stream clock starts with capture.
			extra = fmt.Sprintf(" latency=%dms", time.Since(t0).Milliseconds()-int64(w.End*1000))
		}
		prev = st.Cursor
		printState(ev, st, n, extra)
	}
	if err := <-errc; err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// apiKey reads DEEPGRAM_API_KEY from the environment, falling back to .env.
func apiKey() string {
	if k := os.Getenv("DEEPGRAM_API_KEY"); k != "" {
		return k
	}
	f, err := os.Open(".env")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "DEEPGRAM_API_KEY="); ok {
			return strings.Trim(k, `"'`)
		}
	}
	return ""
}

// recvWords forwards words and prints a one-time hint if none arrive before
// hint fires (Deepgram sends empty results for silence, so nothing prints).
func recvWords(in <-chan stt.Word, hint <-chan time.Time) <-chan stt.Word {
	out := make(chan stt.Word)
	go func() {
		defer close(out)
		for {
			select {
			case w, ok := <-in:
				if !ok {
					return
				}
				hint = nil
				out <- w
			case <-hint:
				hint = nil
				fmt.Fprintln(os.Stderr, "connected, but no speech recognised yet. Check the mic: --list-devices, then --device N")
			}
		}
	}()
	return out
}
