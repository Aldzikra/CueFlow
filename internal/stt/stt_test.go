package stt

import (
	"context"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseResult(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Word
	}{
		{"interim", `{"type":"Results","is_final":false,"channel":{"alternatives":[{"words":[{"word":"halo","start":0.5,"end":0.8},{"word":"dunia","start":0.9,"end":1.2}]}]}}`,
			[]Word{{"halo", false, 0.5, 0.8}, {"dunia", false, 0.9, 1.2}}},
		{"final", `{"type":"Results","is_final":true,"channel":{"alternatives":[{"words":[{"word":"halo","start":0.5,"end":0.8}]}]}}`,
			[]Word{{"halo", true, 0.5, 0.8}}},
		{"metadata ignored", `{"type":"Metadata","request_id":"x"}`, nil},
		{"empty alternatives", `{"type":"Results","channel":{"alternatives":[]}}`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseResult([]byte(tc.in))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	if _, err := ParseResult([]byte("{")); err == nil {
		t.Error("want error on bad JSON")
	}
}

func TestURL(t *testing.T) {
	u := Config{Lang: "en", Keyterms: []string{"Wails"}}.url()
	for _, s := range []string{"model=nova-3", "language=en", "encoding=linear16", "interim_results=true", "keyterm=Wails"} {
		if !strings.Contains(u, s) {
			t.Errorf("%q missing %q", u, s)
		}
	}
	if u := (Config{Lang: "id", Keyterms: []string{"x"}}).url(); strings.Contains(u, "keyterm") {
		t.Errorf("keyterm sent for id: %s", u)
	}
}

func TestRunAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token k" {
			http.Error(w, "no", 401)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		if _, _, err := c.Read(ctx); err != nil { // first audio chunk
			return
		}
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"Results","is_final":true,"channel":{"alternatives":[{"words":[{"word":"halo","start":0.1,"end":0.4}]}]}}`))
		c.Read(ctx) // hold open until client leaves
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	audio, out := make(chan []byte, 1), make(chan Word, 4)
	audio <- []byte{0, 0}
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{APIKey: "k", Lang: "id", Endpoint: "ws" + strings.TrimPrefix(srv.URL, "http")}, audio, out)
	}()
	if w := <-out; w.Word != "halo" || !w.Final {
		t.Errorf("got %+v", w)
	}
	cancel()
	<-done

	// bad key ends Run with an error instead of retrying forever
	err := Run(context.Background(), Config{APIKey: "bad", Lang: "id", Endpoint: "ws" + strings.TrimPrefix(srv.URL, "http")}, audio, out)
	if err == nil {
		t.Error("want unauthorized error")
	}
}
