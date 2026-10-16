# Kickoff — Fase 1: Scaffold, Normalizer, Alignment Engine

> Tempel isi bagian **Prompt** di bawah ke Claude Code. Mulai dalam plan mode, review plan-nya, lalu biarkan dieksekusi.

---

## Prompt

Baca `CLAUDE.md` dan `docs/PRD.md` terlebih dahulu. Kita mengerjakan **Fase 1** saja. Jangan sentuh audio, Deepgram, atau UI Wails di fase ini.

### Tujuan
Membangun inti CueFlow yang sepenuhnya bisa dites offline: normalizer dan alignment engine, plus fixtures dan CLI pemutar fixture.

### Deliverables

1. **Scaffold repo**
   - `go mod init` (module path: tanyakan saya jika belum ada).
   - Struktur folder sesuai `CLAUDE.md` (hanya yang dibutuhkan fase ini: `cmd/cueflow-cli`, `internal/normalize`, `internal/align`, `testdata/`).
   - `.gitignore` (termasuk `.env`), `.env.example` dengan `DEEPGRAM_API_KEY=`.
   - Belum perlu `wails3 init`; itu di Fase 3.

2. **`internal/normalize`**
   - API: `Normalize(text string, lang Lang) []Token`, di mana `Token` menyimpan bentuk kanonik dan rentang index ke teks asli (untuk highlight).
   - Aturan sesuai PRD §5 untuk `id` dan `en`.
   - Table-driven tests, minimal: reduplikasi (`buku-buku`, `buku2`), singkatan (`yg`, `dgn`), angka (`2026`, `25%`, `Rp50.000`), contractions (`don't`, `I'm`, `we'll`), tanda baca dan kapitalisasi.

3. **`internal/align`**
   - Murni: tidak ada I/O, network, atau goroutine.
   - API kira-kira:
     ```go
     type Config struct { Window, MissToImprov, HitToResume, JumpNGram int; WordSim float64 }
     func DefaultConfig() Config
     func New(script []normalize.Token, cfg Config) *Aligner
     func (a *Aligner) Feed(ev WordEvent) AlignState // ev: word, isFinal, start/end time
     func (a *Aligner) State() AlignState
     func (a *Aligner) Seek(cursor int)             // untuk hotkey manual nanti
     ```
   - Implementasikan PRD §4: lookahead window, fuzzy per kata, hysteresis, jump berbasis n-gram unik, rekonsiliasi interim/final, dan prinsip "lebih baik tertahan daripada salah lompat".

4. **Fixtures di `testdata/`**
   - `scripts/id_01.txt`, `scripts/en_01.txt`: masing-masing 4–5 paragraf, gaya bicara YouTuber/presenter, ID boleh mengandung istilah Inggris.
   - `transcripts/*.json`: urutan `WordEvent` (word, isFinal, start, end) yang meniru output STT, untuk skenario:
     - `*_straight`: baca lurus
     - `*_improv`: improvisasi 10–20 kata di tengah, lalu kembali ke skrip
     - `*_skip`: lompat satu paragraf ke depan
     - `*_misheard`: 10–15% kata salah dengar / hilang
     - `*_interim`: interim result yang direvisi sebelum final
     - `id_codeswitch`: kalimat campur ID/EN
   - Setiap fixture punya file ekspektasi (cursor akhir, kapan mode berubah) yang dipakai test.

5. **`cmd/cueflow-cli`**
   - `--script <file> --transcript <file> --lang id|en` lalu memutar transcript ke aligner dan mencetak timeline: waktu, kata, cursor, mode. Berguna untuk tuning manual.

### Kriteria selesai
- `go test ./...` dan `go vet ./...` lulus.
- Semua skenario fixture lulus, termasuk: tidak ada jump palsu di `*_improv` dan `*_misheard`, dan `*_skip` melompat ke paragraf yang benar.
- Default di `align.Config` didokumentasikan beserta alasan nilainya.
- Ringkasan singkat di akhir: apa yang dibangun, keputusan desain yang diambil, dan skenario yang masih lemah.

### Batasan
- Jangan tambah dependensi di luar standard library tanpa bertanya (library Levenshtein kecil boleh diusulkan, tapi implementasi sendiri juga oke).
- Jangan mulai Fase 2.
