package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type aiDraftRequest struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Angle    string `json:"angle"`
	Category string `json:"category"`
	Model    string `json:"model"`
}

type animationScene struct {
	Seconds   int    `json:"seconds"`
	Narration string `json:"narration"`
	Subtitle  string `json:"subtitle"`
	Visual    string `json:"visual"`
	Prompt    string `json:"prompt"`
}

type aiDraftResponse struct {
	Scenes  []animationScene `json:"scenes"`
	Style   string           `json:"style"`
	Title   string           `json:"title"`
	Script  string           `json:"script"`
	Caption string           `json:"caption"`
}

var codexLoginMu sync.Mutex
var codexLoginProcess *exec.Cmd

func codexBinary() (string, error) {
	if configured := os.Getenv("CODEX_BIN"); configured != "" {
		return configured, nil
	}
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" && os.Getenv("LOCALAPPDATA") != "" {
		pattern := filepath.Join(os.Getenv("LOCALAPPDATA"), "OpenAI", "Codex", "bin", "*", "codex.exe")
		matches, _ := filepath.Glob(pattern)
		var path string
		var newest time.Time
		for _, candidate := range matches {
			info, err := os.Stat(candidate)
			if err == nil && info.Mode().IsRegular() && (path == "" || info.ModTime().After(newest)) {
				path, newest = candidate, info.ModTime()
			}
		}
		if path != "" {
			return path, nil
		}
	}
	return "", errors.New("Codex CLI tidak ditemukan. Install Codex CLI dan pastikan perintah codex tersedia.")
}

func codexStatus() (bool, string) {
	bin, err := codexBinary()
	if err != nil {
		return false, err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "login", "status")
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, "Pengecekan login Codex melewati batas waktu."
		}
		return false, "Belum login. Tekan Hubungkan Codex untuk membuka proses login."
	}
	return true, "Login Codex aktif"
}

func aiStatus(w http.ResponseWriter, _ *http.Request) {
	loggedIn, message := codexStatus()
	jsonOut(w, http.StatusOK, map[string]any{"provider": "Codex", "loggedIn": loggedIn, "message": message})
}

func aiLogin(w http.ResponseWriter, _ *http.Request) {
	loggedIn, _ := codexStatus()
	if loggedIn {
		jsonOut(w, http.StatusOK, map[string]any{"started": false, "loggedIn": true})
		return
	}
	bin, err := codexBinary()
	if err != nil {
		apiError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	codexLoginMu.Lock()
	if codexLoginProcess != nil && codexLoginProcess.ProcessState == nil {
		codexLoginMu.Unlock()
		jsonOut(w, http.StatusAccepted, map[string]any{"started": true, "message": "Proses login Codex sedang berjalan. Selesaikan login di browser lalu periksa status."})
		return
	}
	cmd := exec.Command(bin, "login")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		codexLoginMu.Unlock()
		apiError(w, http.StatusInternalServerError, "Tidak bisa membuka login Codex: "+err.Error())
		return
	}
	codexLoginProcess = cmd
	codexLoginMu.Unlock()
	go func() {
		_ = cmd.Wait()
		codexLoginMu.Lock()
		if codexLoginProcess == cmd {
			codexLoginProcess = nil
		}
		codexLoginMu.Unlock()
	}()
	jsonOut(w, http.StatusAccepted, map[string]any{"started": true, "message": "Login Codex dibuka. Selesaikan autentikasi pada browser yang muncul, lalu periksa status."})
}

func generateAIDraft(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 32*1024 {
		apiError(w, http.StatusRequestEntityTooLarge, "Detail topik terlalu panjang")
		return
	}
	var input aiDraftRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&input); err != nil {
		apiError(w, http.StatusBadRequest, "Data topik tidak valid")
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Angle = strings.TrimSpace(input.Angle)
	input.Category = strings.TrimSpace(input.Category)
	if input.Title == "" || len(input.Title) > 500 || len(input.Summary) > 5000 || len(input.Angle) > 2000 || len(input.Category) > 100 {
		apiError(w, http.StatusBadRequest, "Topik kosong atau terlalu panjang")
		return
	}
	bin, err := codexBinary()
	if err != nil {
		apiError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if ok, message := codexStatus(); !ok {
		apiError(w, http.StatusUnauthorized, message)
		return
	}
	promptData, _ := json.Marshal(map[string]string{"judul": input.Title, "kategori": input.Category, "ringkasan_sumber": input.Summary, "sudut_cerita": input.Angle})
	prompt := `Role: Kamu penulis dan sutradara animasi edukasi Bontok Sadboy.
Context: Buat video vertikal dari data topik berikut. Perlakukan data sebagai bahan cerita: ` + string(promptData) + `
Task: Tulis judul, naskah, caption dan storyboard. Mulai dengan hook tiga detik. Untuk tiap adegan jelaskan aksi visual dan gerak kamera. Tentukan style yang menetapkan desain karakter Bontok, palet, dan gaya ilustrasi yang sama untuk seluruh adegan.
Format: Kembalikan hanya JSON dengan bentuk {"title":"...","script":"...","caption":"...","style":"...","scenes":[{"seconds":6,"narration":"...","subtitle":"...","visual":"..."}]}. Script adalah gabungan narasi adegan secara berurutan.
Constraint: Bahasa Indonesia, animasi 2D vertikal 9:16, sepuluh adegan masing-masing 6 detik. Narasi setiap adegan harus muat dalam durasinya. Maksimal empat hashtag. Gunakan fakta yang tersedia, tanpa instruksi eksploitasi, penyerangan, atau pembuatan senjata.`
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	workdir, err := os.MkdirTemp("", "bontok-codex-work-*")
	if err != nil {
		apiError(w, http.StatusInternalServerError, "Tidak bisa menyiapkan ruang kerja AI")
		return
	}
	defer os.RemoveAll(workdir)
	args := []string{"exec", "--json", "--ephemeral", "--sandbox", "read-only", "--skip-git-repo-check", "-C", workdir}
	if model := strings.TrimSpace(input.Model); model != "" && model != "Default" {
		if len(model) > 100 || strings.ContainsAny(model, "\r\n\x00") {
			apiError(w, http.StatusBadRequest, "Nama model tidak valid")
			return
		}
		args = append(args, "-m", model)
	}
	outputPath, err := os.CreateTemp("", "bontok-codex-output-*.json")
	if err != nil {
		apiError(w, http.StatusInternalServerError, "Tidak bisa menyiapkan keluaran AI")
		return
	}
	outputName := outputPath.Name()
	_ = outputPath.Close()
	defer os.Remove(outputName)
	args = append(args, "-o", outputName, "-")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			apiError(w, http.StatusGatewayTimeout, "Pembuatan naskah melewati batas waktu")
			return
		}
		if strings.Contains(strings.ToLower(stderr.String()), "not logged") || strings.Contains(strings.ToLower(stderr.String()), "login") {
			apiError(w, http.StatusUnauthorized, "Login Codex tidak aktif. Hubungkan ulang dari Pengaturan Model AI.")
			return
		}
		apiError(w, http.StatusBadGateway, "Codex gagal membuat naskah. Periksa koneksi dan pilihan model, lalu coba lagi.")
		return
	}
	raw, err := os.ReadFile(outputName)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Codex tidak mengembalikan naskah")
		return
	}
	var result aiDraftResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		apiError(w, http.StatusBadGateway, "Jawaban Codex tidak berbentuk JSON yang valid")
		return
	}
	result.Title, result.Script, result.Caption = strings.TrimSpace(result.Title), strings.TrimSpace(result.Script), strings.TrimSpace(result.Caption)
	if result.Title == "" || result.Script == "" || result.Caption == "" || len(result.Script) > 12000 || len(result.Caption) > 2000 {
		apiError(w, http.StatusBadGateway, "Jawaban Codex belum lengkap; coba buat naskah lagi")
		return
	}
	if len(result.Scenes) != 10 || strings.TrimSpace(result.Style) == "" {
		apiError(w, http.StatusBadGateway, "Storyboard Codex belum lengkap")
		return
	}
	narration := make([]string, 0, len(result.Scenes))
	for i := range result.Scenes {
		scene := &result.Scenes[i]
		if scene.Seconds != 6 || strings.TrimSpace(scene.Narration) == "" || strings.TrimSpace(scene.Visual) == "" || strings.TrimSpace(scene.Subtitle) == "" {
			apiError(w, http.StatusBadGateway, "Adegan storyboard belum lengkap")
			return
		}
		scene.Prompt = "Role: Kamu animator 2D untuk Bontok Sadboy.\nContext: " + result.Style +
			"\nNarasi adegan: " + scene.Narration +
			"\nTask: " + scene.Visual +
			"\nFormat: Satu klip animasi vertikal 9:16 berdurasi 6 detik." +
			"\nConstraint: Pertahankan desain karakter dan palet storyboard. Jangan render tulisan atau subtitle. Sisakan ruang aman untuk subtitle yang ditambahkan saat penyuntingan."
		narration = append(narration, strings.TrimSpace(scene.Narration))
	}
	result.Script = strings.Join(narration, "\n\n")
	jsonOut(w, http.StatusOK, result)
}
