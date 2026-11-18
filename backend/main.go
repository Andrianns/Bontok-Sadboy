package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const maxVideo = 64 * 1024 * 1024

var publishSlot = make(chan struct{}, 1)
var media sync.Map

type tiktokResult struct {
	Error struct{ Code string }
	Data  struct {
		Nickname         string   `json:"creator_nickname"`
		Privacy          []string `json:"privacy_level_options"`
		MaxDuration      int      `json:"max_video_post_duration_sec"`
		CommentsDisabled bool     `json:"comment_disabled"`
		DuetDisabled     bool     `json:"duet_disabled"`
		StitchDisabled   bool     `json:"stitch_disabled"`
		PublishID        string   `json:"publish_id"`
		UploadURL        string   `json:"upload_url"`
		Status           string   `json:"status"`
	}
}

func tik(ctx context.Context, path, access string, body any) (tiktokResult, error) {
	var v tiktokResult
	err := postJSON(ctx, "https://open.tiktokapis.com/v2/post/publish/"+path, access, body, &v)
	if err == nil && v.Error.Code != "ok" {
		err = fmt.Errorf("TikTok menolak permintaan (%s)", v.Error.Code)
	}
	return v, err
}
func creator(w http.ResponseWriter, r *http.Request) {
	t, e := getToken(r.Context(), "tiktok")
	if e != nil {
		apiError(w, 400, e.Error())
		return
	}
	v, e := tik(r.Context(), "creator_info/query/", t.Access, map[string]any{})
	if e != nil {
		apiError(w, 502, e.Error())
		return
	}
	jsonOut(w, 200, v.Data)
}
func youtube(ctx context.Context, access, title, caption, privacy string, video []byte) (string, error) {
	if privacy != "public" && privacy != "unlisted" && privacy != "private" {
		return "", errors.New("Pilih visibilitas YouTube")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata, e := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}})
	if e != nil {
		return "", e
	}
	if e = json.NewEncoder(metadata).Encode(map[string]any{"snippet": map[string]string{"title": title, "description": caption}, "status": map[string]any{"privacyStatus": privacy, "selfDeclaredMadeForKids": false}}); e != nil {
		return "", e
	}
	part, e := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"video/mp4"}})
	if e != nil {
		return "", e
	}
	if _, e = part.Write(video); e != nil {
		return "", e
	}
	if e = writer.Close(); e != nil {
		return "", e
	}
	var out struct{ ID string }
	e = request(ctx, "POST", "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=multipart&part=snippet,status", access, "multipart/related; boundary="+writer.Boundary(), &body, &out)
	if e != nil {
		return "", e
	}
	if out.ID == "" {
		return "", errors.New("YouTube tidak mengembalikan ID video")
	}
	return out.ID, nil
}
func tiktok(ctx context.Context, access, caption, privacy string, video []byte, disableComments, disableDuet, disableStitch bool) (string, error) {
	info, e := tik(ctx, "creator_info/query/", access, map[string]any{})
	if e != nil {
		return "", e
	}
	allowed := false
	for _, p := range info.Data.Privacy {
		if p == privacy {
			allowed = true
		}
	}
	if !allowed {
		return "", errors.New("Pilih visibilitas yang tersedia untuk akun TikTok")
	}
	init, e := tik(ctx, "video/init/", access, map[string]any{
		"post_info":   map[string]any{"title": caption, "privacy_level": privacy, "disable_comment": disableComments || info.Data.CommentsDisabled, "disable_duet": disableDuet || info.Data.DuetDisabled, "disable_stitch": disableStitch || info.Data.StitchDisabled},
		"source_info": map[string]any{"source": "FILE_UPLOAD", "video_size": len(video), "chunk_size": len(video), "total_chunk_count": 1},
	})
	if e != nil {
		return "", e
	}
	u, e := url.Parse(init.Data.UploadURL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", errors.New("TikTok tidak memberikan URL upload yang valid")
	}
	req, e := http.NewRequestWithContext(ctx, "PUT", u.String(), bytes.NewReader(video))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(video)-1, len(video)))
	res, e := client.Do(req)
	if e != nil {
		return "", errors.New("Transfer video TikTok terputus")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Transfer TikTok gagal (HTTP %d)", res.StatusCode)
	}
	return init.Data.PublishID, nil
}
func instagram(ctx context.Context, t token, caption string, video []byte) (string, error) {
	version := os.Getenv("INSTAGRAM_GRAPH_VERSION")
	if !secure || version == "" || t.UserID == "" {
		return "", errors.New("Instagram memerlukan origin HTTPS, versi Graph API dan akun profesional")
	}
	key := random()
	media.Store(key, video)
	defer media.Delete(key)
	endpoint := "https://graph.instagram.com/" + url.PathEscape(version)
	var container struct{ ID string }
	form := url.Values{"media_type": {"REELS"}, "video_url": {origin + "/media/" + key + ".mp4"}, "caption": {caption}}
	e := request(ctx, "POST", endpoint+"/"+url.PathEscape(t.UserID.String())+"/media", t.Access, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), &container)
	if e != nil {
		return "", e
	}
	if container.ID == "" {
		return "", errors.New("Instagram tidak mengembalikan ID container")
	}
	for range 24 {
		var status struct {
			Status string `json:"status_code"`
		}
		e = request(ctx, "GET", endpoint+"/"+url.PathEscape(container.ID)+"?fields=status_code", t.Access, "", nil, &status)
		if e != nil {
			return "", e
		}
		if status.Status == "FINISHED" {
			var out struct{ ID string }
			form = url.Values{"creation_id": {container.ID}}
			e = request(ctx, "POST", endpoint+"/"+url.PathEscape(t.UserID.String())+"/media_publish", t.Access, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), &out)
			if e != nil {
				return "", e
			}
			if out.ID == "" {
				return "", errors.New("Instagram tidak mengembalikan ID Reel")
			}
			return out.ID, nil
		}
		if status.Status == "ERROR" || status.Status == "EXPIRED" {
			return "", errors.New("Instagram gagal memproses video")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return "", errors.New("Instagram belum selesai memproses video dalam batas waktu")
}
func publish(w http.ResponseWriter, r *http.Request) {
	select {
	case publishSlot <- struct{}{}:
		defer func() { <-publishSlot }()
	default:
		apiError(w, 429, "Tunggu unggahan sebelumnya selesai")
		return
	}
	p := r.PathValue("platform")
	if !configured(p) {
		apiError(w, 503, "Platform belum dikonfigurasi")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVideo+1024*1024)
	if e := r.ParseMultipartForm(8 << 20); e != nil {
		apiError(w, 400, "Form upload tidak valid atau lebih dari 64 MiB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, e := r.FormFile("video")
	if e != nil {
		apiError(w, 400, "Pilih file MP4")
		return
	}
	defer f.Close()
	video, e := io.ReadAll(io.LimitReader(f, maxVideo+1))
	if e != nil || len(video) < 12 || len(video) > maxVideo || string(video[4:8]) != "ftyp" {
		apiError(w, 400, "Gunakan video MP4 yang valid, maksimal 64 MiB")
		return
	}
	if r.FormValue("consent") != "yes" {
		apiError(w, 400, "Konfirmasi unggahan terlebih dahulu")
		return
	}
	t, e := getToken(r.Context(), p)
	if e != nil {
		apiError(w, 400, e.Error())
		return
	}
	title, caption, privacy := strings.TrimSpace(r.FormValue("title")), r.FormValue("caption"), r.FormValue("privacy")
	var id string
	switch p {
	case "youtube":
		id, e = youtube(r.Context(), t.Access, title, caption, privacy, video)
	case "tiktok":
		id, e = tiktok(r.Context(), t.Access, caption, privacy, video, r.FormValue("disable_comment") != "false", r.FormValue("disable_duet") != "false", r.FormValue("disable_stitch") != "false")
	case "instagram":
		id, e = instagram(r.Context(), t, caption, video)
	}
	if e != nil {
		apiError(w, 502, e.Error())
		return
	}
	status := "Terkirim"
	if p == "tiktok" {
		status = "Diproses"
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": status})
}
func publicationStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" || len(id) > 200 {
		apiError(w, 400, "ID publikasi tidak valid")
		return
	}
	t, e := getToken(r.Context(), "tiktok")
	if e != nil {
		apiError(w, 400, e.Error())
		return
	}
	v, e := tik(r.Context(), "status/fetch/", t.Access, map[string]string{"publish_id": id})
	if e != nil {
		apiError(w, 502, e.Error())
		return
	}
	status := "Diproses"
	switch v.Data.Status {
	case "PUBLISH_COMPLETE":
		status = "Terbit"
	case "FAILED":
		status = "Gagal"
	}
	jsonOut(w, 200, map[string]string{"status": status})
}
func main() {
	recoverPluginJobs()
	if secure && len(os.Getenv("APP_PASSWORD")) < 16 {
		log.Fatal("APP_PASSWORD minimal 16 karakter untuk origin HTTPS")
	}
	if e := loadVault(); e != nil {
		log.Fatal("Tidak bisa membaca token store: ", e)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/news", func(w http.ResponseWriter, r *http.Request) {
		data := refreshNews(false)
		status := 200
		if len(data.Topics) == 0 && data.FailedSources > 0 {
			status = 503
		}
		jsonOut(w, status, data)
	})
	mux.HandleFunc("GET /api/ai/status", aiStatus)
	mux.HandleFunc("POST /api/ai/login", aiLogin)
	mux.HandleFunc("POST /api/ai/draft", generateAIDraft)
	mux.HandleFunc("POST /api/render/clip", renderClip)
	mux.HandleFunc("GET /api/render/ready", animationReady)
	mux.HandleFunc("POST /api/render/assemble", assembleAnimation)
	mux.HandleFunc("GET /api/render/clip/{id}", renderClip)
	mux.HandleFunc("POST /api/news/refresh", func(w http.ResponseWriter, r *http.Request) {
		data := refreshNews(true)
		status := 200
		if len(data.Topics) == 0 && data.FailedSources > 0 {
			status = 503
		}
		jsonOut(w, status, data)
	})
	mux.HandleFunc("GET /api/accounts", accounts)
	mux.HandleFunc("GET /api/oauth/{platform}/start", oauthStart)
	mux.HandleFunc("GET /api/oauth/{platform}/callback", oauthCallback)
	mux.HandleFunc("DELETE /api/accounts/{platform}", disconnect)
	mux.HandleFunc("GET /api/tiktok/creator", creator)
	mux.HandleFunc("GET /api/tiktok/status", publicationStatus)
	mux.HandleFunc("POST /api/publish/{platform}", publish)
	mux.Handle("/", http.FileServer(http.Dir(env("STATIC_DIR", "../dist"))))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if (r.Method == "GET" || r.Method == "HEAD") && strings.HasPrefix(r.URL.Path, "/media/") {
			key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/media/"), ".mp4")
			if v, ok := media.Load(key); ok {
				w.Header().Set("Content-Type", "video/mp4")
				w.Header().Set("Cache-Control", "no-store")
				http.ServeContent(w, r, "video.mp4", time.Time{}, bytes.NewReader(v.([]byte)))
				return
			}
			http.NotFound(w, r)
			return
		}
		if password := os.Getenv("APP_PASSWORD"); password != "" {
			user, pass, ok := r.BasicAuth()
			if !ok || !same(user, env("APP_USERNAME", "bontok")) || !same(pass, password) {
				w.Header().Set("WWW-Authenticate", `Basic realm="Bontok Studio"`)
				http.Error(w, "Login studio diperlukan", 401)
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != origin && (!secure && r.Header.Get("Origin") != "http://127.0.0.1:5174" || secure) {
			apiError(w, 403, "Origin tidak diizinkan")
			return
		}
		mux.ServeHTTP(w, r)
	})
	go func() {
		refreshNews(false)
		tick := time.NewTicker(30 * time.Minute)
		defer tick.Stop()
		for range tick.C {
			refreshNews(false)
		}
	}()
	addr := env("APP_LISTEN", "127.0.0.1:8080")
	host, _, e := net.SplitHostPort(addr)
	if e != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
		log.Fatal("Backend harus bind ke loopback. Gunakan reverse proxy HTTPS untuk akses luar.")
	}
	log.Print("Bontok Studio: ", addr)
	log.Fatal((&http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, ReadTimeout: 3 * time.Minute, WriteTimeout: 5 * time.Minute}).ListenAndServe())
}
