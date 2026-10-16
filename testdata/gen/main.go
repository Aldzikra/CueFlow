// Command gen regenerates testdata/transcripts from testdata/scripts.
// Run from the repo root: go run ./testdata/gen
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/Aldzikra/cueflow/internal/align"
	"github.com/Aldzikra/cueflow/internal/normalize"
)

type expect struct {
	Script      string   `json:"script"`
	Lang        string   `json:"lang"`
	FinalCursor int      `json:"finalCursor"`
	Modes       []string `json:"modes"` // distinct consecutive modes, idle omitted
}

var improvWords = map[string]string{
	"id": "ceritanya lucu banget tadi aku ketemu tukang bakso depan rumah ngakak terus dia bilang kopi",
	"en": "oh wait funny story my neighbor dog just barked whole time anyway laughed pizza yesterday",
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	for _, lang := range []string{"id", "en"} {
		raw, err := os.ReadFile(fmt.Sprintf("testdata/scripts/%s_01.txt", lang))
		must(err)
		text := string(raw)
		toks := normalize.Normalize(text, normalize.Lang(lang))
		words := make([]string, len(toks))
		for i, t := range toks {
			words[i] = t.Text
		}
		// paragraph start = first token after a blank line
		paras := []int{0}
		for i := 1; i < len(toks); i++ {
			if toks[i-1].End <= toks[i].Start && strings.Contains(text[toks[i-1].End:toks[i].Start], "\n\n") {
				paras = append(paras, i)
			}
		}
		n := len(words)
		rng := rand.New(rand.NewSource(42))
		name := lang + "_01"
		script := fmt.Sprintf("scripts/%s_01.txt", lang)

		emit := func(file string, evs []align.WordEvent, modes ...string) {
			b, _ := json.MarshalIndent(evs, "", " ")
			must(os.WriteFile("testdata/transcripts/"+file+".json", b, 0o644))
			e, _ := json.MarshalIndent(expect{script, lang, n, modes}, "", " ")
			must(os.WriteFile("testdata/transcripts/"+file+".expect.json", e, 0o644))
		}
		stream := func(ws []string, final bool, t *float64) []align.WordEvent {
			var out []align.WordEvent
			for _, w := range ws {
				d := 0.2 + rng.Float64()*0.25
				out = append(out, align.WordEvent{Word: w, IsFinal: final, Start: round(*t), End: round(*t + d)})
				*t += d + 0.05
			}
			return out
		}
		cat := func(parts ...[]string) []string {
			var o []string
			for _, p := range parts {
				o = append(o, p...)
			}
			return o
		}
		improv := strings.Fields(improvWords[lang])

		var t float64
		emit(name+"_straight", stream(words, true, &t), "tracking")

		t = 0
		a := paras[1] + 12
		emit(name+"_improv", stream(cat(words[:a], improv, words[a:]), true, &t), "tracking", "improv", "tracking")

		t = 0
		emit(name+"_skip", stream(cat(words[:paras[1]+10], words[paras[3]:]), true, &t), "tracking", "jumped", "tracking")
		// cursor must end at n: the spoken stream ends with the script's last word

		t = 0
		var heard []string
		for i, w := range words {
			if i < 3 || i >= n-5 || rng.Float64() > 0.12 {
				heard = append(heard, w)
				continue
			}
			switch {
			case rng.Float64() < 0.4: // dropped
			case len([]rune(w)) >= 5: // one char lost
				heard = append(heard, w[:len(w)-2]+w[len(w)-1:])
			default:
				heard = append(heard, w+"x")
			}
		}
		emit(name+"_misheard", stream(heard, true, &t), "tracking")

		// interim: chunks of 5; first word of each chunk is first heard as a
		// word from further ahead, then revised, then all five finalised.
		t = 0
		var evs []align.WordEvent
		for i := 0; i < n; i += 5 {
			chunk := words[i:min(i+5, n)]
			t0 := t
			evs = append(evs, stream(chunk[:1], false, &t)...)
			if i+7 < n {
				evs[len(evs)-1].Word = words[i+7]
				evs = append(evs, evs[len(evs)-1])
				evs[len(evs)-1].Word = chunk[0]
			}
			evs = append(evs, stream(chunk[1:], false, &t)...)
			t = t0
			evs = append(evs, stream(chunk, true, &t)...)
		}
		emit(name+"_interim", evs, "tracking")

		if lang == "id" {
			sw := map[string]string{"channel": "chanel", "subscribe": "sabskraib", "video": "vidio", "videomu": "vidiomu"}
			var cs []string
			for _, w := range words {
				if r, ok := sw[w]; ok {
					w = r
				}
				cs = append(cs, w)
			}
			t = 0
			emit("id_codeswitch", stream(cs, true, &t), "tracking")
		}
	}
}

func round(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }
