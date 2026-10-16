# Product Requirement Document — CueFlow

**Produk:** CueFlow, adaptive teleprompter
**Platform:** Desktop Linux (X11; Wayland via XWayland untuk MVP)
**Stack:** Go + Wails v3, React/TypeScript
**Status:** v1.2 (revisi setelah review teknis)

---

## 1. Ringkasan

CueFlow adalah teleprompter yang **posisinya digerakkan oleh suara**. Kursor skrip hanya maju ketika kata yang diucapkan cocok dengan skrip. Saat pembicara improvisasi, kursor diam di tempat dan muncul indikator halus "Off-Script". Begitu pembicara kembali ke skrip, termasuk jika melompati paragraf, tampilan menyusul secara otomatis.

Perubahan utama dari v1.1:
- Model "auto-scroll lalu pause" diganti dengan **voice-driven cursor**. Pause saat improvisasi terjadi secara alami.
- Matching dilakukan **per kata dengan search window dan hysteresis**, bukan perbandingan kalimat utuh.
- Ditambahkan manual override (hotkey), fallback saat STT terputus, dan catatan keterbatasan Wayland.

## 2. Target pengguna

- **Content creator / YouTuber:** menyelipkan humor atau cerita spontan.
- **Presenter / public speaker:** menghadapi interupsi dan pertanyaan live.
- **Sales & demo engineer:** pitching berbasis skrip sambil menjawab pertanyaan.

## 3. Fitur inti

### 3.1 Multibahasa (ID & EN)
- Bahasa skrip dipilih sebelum sesi dimulai: `id` atau `en`.
- Pilihan bahasa menentukan parameter STT dan aturan normalisasi.
- Normalisasi diterapkan **ke skrip dan transkrip dengan fungsi yang sama** (lihat §5).

### 3.2 Audio & STT
- Capture mikrofon di Go (malgo, via PipeWire/PulseAudio), mono 16 kHz linear16.
- Streaming ke Deepgram Nova-3 via WebSocket, `interim_results=true`.
- Jika tersedia untuk bahasa terkait, kirim kosakata unik dari skrip (nama, istilah) sebagai keyword/keyterm boosting.
- Reconnect otomatis. Jika koneksi gagal lebih dari N detik, masuk mode fallback (§3.5).

### 3.3 Alignment engine
Lihat spesifikasi lengkap di §4.
- **Tracking:** kursor maju per kata yang cocok di dalam window di depan kursor.
- **Improv:** setelah beberapa kata berturut-turut tidak cocok, status berubah menjadi improv; kursor tetap.
- **Resume:** setelah beberapa kata berturut-turut cocok, kembali ke tracking.
- **Jump:** jika pembicara melompat ke bagian lain, kursor melompat hanya bila ada bukti kuat (n-gram match).

### 3.4 Overlay & UI
- **Window editor:** tempel/edit skrip, pilih mic, pilih bahasa, atur tampilan.
- **Window overlay:** frameless, transparan, always-on-top; menyorot kata aktif; baris aktif di posisi tetap (misalnya sepertiga atas).
- Indikator improv yang halus di sudut overlay.
- Pengaturan: ukuran font, line height, opacity, lebar, mirror mode (horizontal flip), posisi baris aktif.

### 3.5 Kontrol manual & fallback
- **Global hotkeys:** maju satu kalimat, mundur satu kalimat, pause/resume tracking, toggle overlay.
- **Fallback:** saat STT tidak tersedia, overlay beralih ke scroll manual (hotkey) atau kecepatan tetap yang bisa diatur, dengan indikator status koneksi.

## 4. Spesifikasi alignment

**Input:** daftar token skrip ternormalisasi (dengan index stabil) dan stream kata dari STT (interim dan final, dengan timestamp).
**Output:** `AlignState { cursor, mode, confidence, tentativeUntil, lang }`.

1. **Lookahead window.** Setiap kata baru dicocokkan ke `W` kata di depan kursor (default `W = 15`). Kata yang terlewat di antara kursor dan posisi match dianggap sudah dibaca (toleran terhadap kata yang tidak terdengar STT).
2. **Fuzzy per kata.** Kecocokan kata memakai similarity ternormalisasi (misalnya Levenshtein ratio ≥ 0.8, threshold bisa diatur) untuk menangani salah dengar STT.
3. **Hysteresis.**
   - `tracking → improv` setelah `M` kata berturut-turut tidak cocok (default `M = 4`).
   - `improv → tracking` setelah `R` kata berturut-turut cocok di window (default `R = 3`).
4. **Jump.** Pencarian global di seluruh skrip dilakukan hanya ketika sedang improv atau terjadi miss beruntun. Jump hanya dieksekusi jika ada n-gram berurutan dengan panjang ≥ `J` (default `J = 4`) yang cocok kuat dan unik. Jika ada lebih dari satu kandidat, pilih yang terdekat ke depan dari kursor; jika ambigu, jangan lompat.
5. **Interim vs final.** Match dari interim result menggerakkan kursor secara tentatif (`tentativeUntil`). Saat final tiba, lakukan rekonsiliasi. Kursor tidak boleh terlihat mundur akibat koreksi interim kecuali selisihnya besar dan didukung final.
6. **Prinsip:** lebih baik kursor tertahan daripada melompat ke tempat yang salah.

Semua angka default disimpan di `align.Config` dan akan dikalibrasi memakai fixtures.

## 5. Normalisasi

Diterapkan identik untuk skrip dan transkrip:
- Lowercase, hapus tanda baca, normalisasi whitespace, diacritics-insensitive.
- **ID:** reduplikasi (`buku-buku`, `buku2`, `buku buku` → bentuk kanonik), singkatan umum (`yg`, `dgn`, `tdk`, `utk`, `krn`), angka ↔ kata (`2026` ↔ `dua ribu dua puluh enam`), persen dan mata uang dasar (`Rp`).
- **EN:** contractions (`don't` ↔ `do not`), angka ↔ kata, ordinal.
- Satu token skrip boleh berkorespondensi dengan beberapa token ucapan (misalnya angka) — normalizer menghasilkan bentuk kanonik dan mapping ke index token asli untuk highlight.

## 6. Arsitektur

```
[Mic: PipeWire/PulseAudio]
          │ PCM 16 kHz
          ▼
   internal/audio (malgo)
          │
          ▼
   internal/stt ──WebSocket (language=id|en, interim)──► Deepgram Nova-3
          │◄──────────── transcript (interim/final) ─────────┘
          ▼
   internal/normalize ─► internal/align (pure Go)
          │ AlignState
          ▼
   Wails events ─► React overlay + editor
```

## 7. Functional requirements

- [ ] **FR-01** Pengguna dapat menempel/mengedit skrip teks.
- [ ] **FR-02** Pengguna dapat memilih perangkat mikrofon.
- [ ] **FR-03** Pengguna dapat memilih bahasa skrip (ID/EN), yang mengatur STT dan normalisasi.
- [ ] **FR-04** Overlay transparan, frameless, always-on-top.
- [ ] **FR-05** Kursor maju saat ucapan cocok dengan skrip; kata aktif disorot.
- [ ] **FR-06** Kursor tertahan dan indikator improv muncul saat pembicara keluar skrip (dengan hysteresis).
- [ ] **FR-07** Kursor melompat ke bagian yang sesuai saat pembicara melompati paragraf (dengan syarat n-gram).
- [ ] **FR-08** Global hotkeys untuk maju/mundur/pause/toggle overlay.
- [ ] **FR-09** Fallback ke scroll manual/kecepatan tetap saat STT tidak tersedia, dengan indikator status.
- [ ] **FR-10** Pengaturan tampilan: font, opacity, mirror, posisi baris aktif.
- [ ] **FR-11** API key disimpan lokal dan tidak pernah ditampilkan ulang dalam plaintext di UI.

## 8. Non-functional requirements

- **Latency:** dari kata diucapkan hingga kursor bergerak < 400 ms (target), memakai interim results.
- **Memori:** target < 150 MB total PSS (termasuk proses WebKitGTK). Diukur, bukan dijamin.
- **CPU:** idle mendekati nol; saat sesi berjalan, rendah dan stabil.
- **Kompatibilitas:** Ubuntu, Debian, Fedora, Arch. X11 penuh. Wayland via XWayland untuk MVP.
- **Privasi:** audio hanya dikirim ke provider STT selama sesi aktif; tidak ada penyimpanan audio kecuali pengguna mengaktifkan rekaman debug.

## 9. Batasan yang diketahui

- **Wayland:** xdg-shell tidak menyediakan always-on-top. GNOME tidak mendukung `wlr-layer-shell`. MVP berjalan via XWayland; dukungan native per compositor (KDE, Hyprland, Sway, COSMIC) adalah post-MVP dan best effort.
- **Transparansi window** bergantung pada compositor.
- **Code-switching** ID/EN bisa menurunkan akurasi model monolingual; perlu diuji dengan rekaman nyata.

## 10. Roadmap

| Fase | Isi | Kriteria selesai |
|---|---|---|
| 1 | Scaffold, `normalize`, `align` + fixtures | Semua skenario fixture lulus test |
| 2 | Audio + Deepgram + CLI live | CLI mencetak kursor live; latency terukur |
| 3 | Overlay Wails (XWayland) | Overlay jalan di GNOME & KDE, highlight sinkron |
| 4 | Hotkeys, settings, fallback | FR-08 s.d. FR-11 terpenuhi |
| Post-MVP | Native Wayland layer-shell, STT lokal (whisper.cpp/Vosk), skrip multi-file | — |
