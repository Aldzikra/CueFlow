# CLAUDE.md — CueFlow

Voice-driven teleprompter for Linux desktop. The script cursor advances only when spoken words match the script; when the speaker improvises, the cursor stays put and an "Off-Script" indicator appears. Supports Bahasa Indonesia (`id`) and English (`en`).

Full requirements: `docs/PRD.md`. Current phase and task: `docs/KICKOFF.md` (or the latest task given in chat).

## Locked technical decisions

Do not change these without asking first. Each has a reason.

| Area | Decision | Why |
|---|---|---|
| Desktop framework | **Wails v3** (beta), React + TypeScript frontend | First-class multi-window (editor + overlay), global shortcuts incl. Wayland portal |
| STT | **Deepgram Nova-3** streaming over WebSocket, `language=id` or `en`, `interim_results=true` | Indonesian streaming support; interim results needed for < 400 ms latency |
| Audio capture | **`github.com/gen2brain/malgo`** in Go, mono, 16 kHz, `linear16` | WebKitGTK `getUserMedia` is unreliable; malgo works on PipeWire via pipewire-pulse |
| Overlay on Wayland | Run under **XWayland** (`GDK_BACKEND=x11`) for MVP | xdg-shell has no always-on-top; GNOME has no layer-shell. Native Wayland is post-MVP |
| Alignment model | **Voice-driven cursor**, word-level, with hysteresis | Pausing on improvisation falls out naturally; no fixed-speed scrolling |
| Normalization | One shared normalizer applied to **both** script and transcript | Matching only works on a single canonical form |

## Architecture

```
mic ─► internal/audio ─► internal/stt (Deepgram WS) ─► transcript events
                                                         │
                       internal/normalize ◄──────────────┤
                                                         ▼
                                              internal/align (pure Go)
                                                         │  AlignState
                                                         ▼
                                     app service ─► Wails events ─► frontend overlay
```

## Repository layout

```
cmd/cueflow-cli/      # headless CLI: mic or fixture → prints cursor state (dev tool)
internal/normalize/   # tokenization + canonical form, per language
internal/align/       # alignment engine — NO imports of audio, stt, wails
internal/audio/       # malgo capture, device listing
internal/stt/         # Deepgram client, reconnect, interim/final handling
internal/app/         # Wails services wiring everything together
frontend/             # React + TS (editor window, overlay window)
testdata/scripts/     # sample scripts (id, en)
testdata/transcripts/ # simulated STT word streams (JSON)
testdata/audio/       # real recordings (.wav), may be git-lfs
docs/                 # PRD, kickoff, design notes
```

## Commands

```bash
go test ./...                      # all Go tests — must pass before finishing any task
go test ./internal/align/ -run X -v
go vet ./...
wails3 dev                         # run app with hot reload
GDK_BACKEND=x11 wails3 dev         # run under XWayland (required for overlay testing on Wayland)
wails3 build
go run ./cmd/cueflow-cli --script testdata/scripts/id_01.txt --transcript testdata/transcripts/id_01_improv.json
go run ./testdata/gen              # regenerate testdata/transcripts from scripts
go run ./cmd/cueflow-cli --list-devices
go run ./cmd/cueflow-cli --script testdata/scripts/id_01.txt --live --lang id [--device N]   # needs DEEPGRAM_API_KEY
```

Update this section if commands change.

## Working rules

1. **`internal/align` and `internal/normalize` are pure.** No network, no audio, no Wails, no goroutines inside the core algorithm. Input = tokens + events, output = state. This keeps them fully testable offline.
2. **Every change to `align/` or `normalize/` needs table-driven tests**, using fixtures in `testdata/` where possible. Run `go test ./...` before declaring done.
3. **Never commit secrets.** API key comes from `DEEPGRAM_API_KEY` (env or `.env`, which is gitignored). `.env.example` documents it.
4. **Never require a live mic or network in unit tests.** Integration tests that hit Deepgram go behind the build tag `integration`.
5. **Interim transcripts are revisable.** The aligner must treat interim words as tentative and reconcile when the final arrives; the cursor must not visibly jump backward from interim corrections.
6. **Prefer a stuck cursor over a wrong jump.** When unsure, don't move.
7. Keep tunables (window size, thresholds, hysteresis counts) in a single `align.Config` struct with documented defaults; no magic numbers in logic.
8. Go: standard library first, `gofmt`, errors wrapped with `%w`, context-aware cancellation for long-running goroutines.
9. Frontend: TypeScript strict mode; no business logic in React — it renders `AlignState` it receives.
10. Ask before adding a new dependency that isn't listed here.

## Key data contract

Backend → frontend event `align:state`:

```ts
type AlignState = {
  cursor: number;          // index of next expected script word
  mode: "tracking" | "improv" | "jumped" | "idle";
  confidence: number;      // 0..1, recent match quality
  tentativeUntil?: number; // words matched only by interim results
  lang: "id" | "en";
};
```

Script is sent once as tokens with stable indices; the frontend never re-tokenizes.

## Language notes

- Indonesian: reduplication (`buku-buku`, `buku2`, `buku buku` → one canonical form), common abbreviations (`yg`, `dgn`, `tdk`), numbers spoken as words (`dua ribu dua puluh enam` ↔ `2026`). Expect English code-switching mid-sentence.
- English: contractions (`don't` ↔ `do not`), numbers, ordinals.
- Strip punctuation and casing; keep diacritics-insensitive comparison.
