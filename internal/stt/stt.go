// Package stt streams PCM audio to Deepgram Nova-3 over WebSocket and emits
// word events (interim and final). It reconnects with backoff.
package stt

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Word is one recognised word. Times are seconds since the first audio sent
// (continuous across reconnects).
type Word struct {
	Word       string
	Final      bool
	Start, End float64
}

type Config struct {
	APIKey string
	Lang   string // "id" | "en"
	// Keyterms boost vocabulary. Nova-3 `keyterm` is only sent for "en";
	// ponytail: unverified for "id", enable once checked against real audio.
	Keyterms   []string
	Endpoint   string // default wss://api.deepgram.com/v1/listen
	SampleRate int    // default 16000
	// Logf receives connection status lines (optional).
	Logf func(format string, args ...any)
}

func (c Config) url() string {
	ep := c.Endpoint
	if ep == "" {
		ep = "wss://api.deepgram.com/v1/listen"
	}
	rate := c.SampleRate
	if rate == 0 {
		rate = 16000
	}
	q := url.Values{
		"model": {"nova-3"}, "language": {c.Lang}, "encoding": {"linear16"},
		"sample_rate": {fmt.Sprint(rate)}, "channels": {"1"}, "interim_results": {"true"},
	}
	if c.Lang == "en" {
		for _, k := range c.Keyterms {
			q.Add("keyterm", k)
		}
	}
	return ep + "?" + q.Encode()
}

func (c Config) logf(f string, a ...any) {
	if c.Logf != nil {
		c.Logf(f, a...)
	}
}

type result struct {
	Type    string `json:"type"`
	IsFinal bool   `json:"is_final"`
	Channel struct {
		Alternatives []struct {
			Words []struct {
				Word  string  `json:"word"`
				Start float64 `json:"start"`
				End   float64 `json:"end"`
			} `json:"words"`
		} `json:"alternatives"`
	} `json:"channel"`
}

// ParseResult extracts words from one Deepgram message. Non-Results messages
// (Metadata, SpeechStarted, ...) yield nil.
func ParseResult(msg []byte) ([]Word, error) {
	var r result
	if err := json.Unmarshal(msg, &r); err != nil {
		return nil, fmt.Errorf("parse deepgram message: %w", err)
	}
	if r.Type != "Results" || len(r.Channel.Alternatives) == 0 {
		return nil, nil
	}
	var ws []Word
	for _, w := range r.Channel.Alternatives[0].Words {
		ws = append(ws, Word{w.Word, r.IsFinal, w.Start, w.End})
	}
	return ws, nil
}

// Run streams audio until ctx is cancelled or audio is closed, sending words
// on out. Connection errors trigger reconnect with exponential backoff
// (0.5s .. 5s); only ctx cancellation or a bad API key end it with an error.
func Run(ctx context.Context, cfg Config, audio <-chan []byte, out chan<- Word) error {
	var offset float64
	backoff := 500 * time.Millisecond
	for {
		start := time.Now()
		end, err := session(ctx, cfg, audio, out, offset)
		offset = max(offset, end)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err == nil: // audio closed
			return nil
		case errors.Is(err, errUnauthorized):
			return err
		}
		if time.Since(start) > 10*time.Second {
			backoff = 500 * time.Millisecond
		}
		cfg.logf("stt: %v; reconnecting in %s", err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

var errUnauthorized = errors.New("deepgram rejected API key (check DEEPGRAM_API_KEY)")

// session runs one connection; returns the highest word end time seen (absolute).
func session(ctx context.Context, cfg Config, audio <-chan []byte, out chan<- Word, offset float64) (float64, error) {
	conn, resp, err := websocket.Dial(ctx, cfg.url(), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Token " + cfg.APIKey}},
	})
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return offset, errUnauthorized
		}
		return offset, fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	cfg.logf("stt: connected")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	werr := make(chan error, 1)
	go func() { // audio -> socket
		for {
			select {
			case <-ctx.Done():
				werr <- nil
				return
			case b, ok := <-audio:
				if !ok {
					_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"CloseStream"}`))
					werr <- nil
					return
				}
				if err := conn.Write(ctx, websocket.MessageBinary, b); err != nil {
					werr <- fmt.Errorf("write audio: %w", err)
					return
				}
			}
		}
	}()

	last := offset
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			cancel()
			if e := <-werr; e != nil {
				err = e
			}
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return last, nil
			}
			return last, fmt.Errorf("read: %w", err)
		}
		words, err := ParseResult(msg)
		if err != nil {
			continue // skip malformed message
		}
		for _, w := range words {
			w.Start, w.End = w.Start+offset, w.End+offset
			last = max(last, w.End)
			select {
			case out <- w:
			case <-ctx.Done():
			}
		}
	}
}

// APIKey reads DEEPGRAM_API_KEY from the environment, falling back to ./.env.
func APIKey() string {
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
