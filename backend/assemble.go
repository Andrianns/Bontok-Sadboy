package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func animationEncoder() (string, error) {
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}
	platform := runtime.GOOS
	binary := "ffmpeg"
	if platform == "windows" {
		platform, binary = "win32", "ffmpeg.exe"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return exec.LookPath(filepath.Join("..", "node_modules", "@ffmpeg-installer", platform+"-"+arch, binary))
}

func animationReady(w http.ResponseWriter, _ *http.Request) {
	if ok, message := codexStatus(); !ok {
		apiError(w, 503, message)
		return
	}
	if _, err := animationEncoder(); err != nil {
		apiError(w, 503, "FFmpeg belum tersedia. Jalankan npm ci atau pasang FFmpeg di PATH backend.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func downloadAnimationClip(ctx context.Context, rawURL, path string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("alamat klip Higgsfield tidak valid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unduhan klip gagal (HTTP %d)", res.StatusCode)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	const maxClipSize = 64 << 20
	n, copyErr := io.Copy(file, io.LimitReader(res.Body, maxClipSize+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n == 0 || n > maxClipSize {
		return fmt.Errorf("ukuran atau isi klip tidak valid")
	}
	return nil
}

var assembleSlot = make(chan struct{}, 1)

func assembleAnimation(w http.ResponseWriter, r *http.Request) {
	encoder, err := animationEncoder()
	if err != nil {
		apiError(w, 503, "Backend memerlukan FFmpeg.")
		return
	}
	encoder, err = filepath.Abs(encoder)
	if err != nil {
		apiError(w, 500, "Lokasi FFmpeg tidak valid")
		return
	}
	var input []struct {
		ID       string `json:"id"`
		Subtitle string `json:"subtitle"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 12000)).Decode(&input) != nil || len(input) != 10 {
		apiError(w, 400, "Video memerlukan sepuluh adegan")
		return
	}
	seen := make(map[string]bool, len(input))
	for _, scene := range input {
		if !renderID.MatchString(scene.ID) || seen[scene.ID] || len(scene.Subtitle) > 500 {
			apiError(w, 400, "Data adegan tidak valid")
			return
		}
		seen[scene.ID] = true
	}
	select {
	case assembleSlot <- struct{}{}:
		defer func() { <-assembleSlot }()
	default:
		apiError(w, 429, "Video lain sedang digabungkan")
		return
	}
	dir, err := os.MkdirTemp("", "bontok-animation-")
	if err != nil {
		apiError(w, 500, "Tidak bisa menyiapkan penggabungan video")
		return
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	var manifest, subtitles strings.Builder
	for i, scene := range input {
		result, err := readPluginJob(scene.ID)
		if err != nil || result.Status != "completed" || result.Video == nil {
			apiError(w, 502, fmt.Sprintf("Hasil adegan %d belum bisa diambil", i+1))
			return
		}
		name := fmt.Sprintf("scene-%02d.mp4", i)
		if err := downloadAnimationClip(ctx, result.Video.URL, filepath.Join(dir, name)); err != nil {
			apiError(w, 502, fmt.Sprintf("Klip adegan %d gagal diunduh dari Higgsfield", i+1))
			return
		}
		fmt.Fprintf(&manifest, "file '%s'\n", name)
		start, end := i*6, (i+1)*6
		fmt.Fprintf(&subtitles, "%d\n00:%02d:%02d,000 --> 00:%02d:%02d,000\n%s\n\n",
			i+1, start/60, start%60, end/60, end%60, strings.Join(strings.Fields(scene.Subtitle), " "))
	}
	if os.WriteFile(filepath.Join(dir, "clips.txt"), []byte(manifest.String()), 0600) != nil ||
		os.WriteFile(filepath.Join(dir, "captions.srt"), []byte(subtitles.String()), 0600) != nil {
		apiError(w, 500, "File penggabungan gagal dibuat")
		return
	}
	cmd := exec.CommandContext(ctx, encoder, "-nostdin", "-loglevel", "error", "-f", "concat", "-safe", "1",
		"-i", "clips.txt", "-map", "0:v:0", "-map", "0:a:0", "-vf", "subtitles=captions.srt",
		"-c:a", "aac", "-b:a", "128k", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-b:v", "5M", "-maxrate", "6M", "-bufsize", "12M", "-movflags", "+faststart", "animation.mp4")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		apiError(w, 502, "FFmpeg gagal menggabungkan klip, suara, dan subtitle. Periksa apakah hasil render memiliki audio.")
		return
	}
	final := filepath.Join(dir, "animation.mp4")
	info, err := os.Stat(final)
	if err != nil || info.Size() == 0 || info.Size() > maxVideo {
		apiError(w, 502, "Ukuran video akhir tidak didukung untuk publikasi")
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, final)
}
