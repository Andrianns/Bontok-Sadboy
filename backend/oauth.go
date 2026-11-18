package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var origin = strings.TrimRight(env("APP_ORIGIN", "http://127.0.0.1:8080"), "/")
var secure = strings.HasPrefix(origin, "https://")
var client = &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
var vault struct {
	sync.Mutex
	Tokens map[string]token
}
var stateDir = env("STATE_DIR", ".state")
var platformNames = []string{"youtube", "tiktok", "instagram"}

type provider struct{ authorize, exchange, idKey, id, secret, scope string }
type token struct {
	Access    string      `json:"access_token"`
	Refresh   string      `json:"refresh_token"`
	ExpiresIn int64       `json:"expires_in"`
	Expires   time.Time   `json:"expires"`
	UserID    json.Number `json:"user_id"`
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func random() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func oauth(p string) provider {
	switch p {
	case "youtube":
		return provider{"https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", "client_id", os.Getenv("YOUTUBE_CLIENT_ID"), os.Getenv("YOUTUBE_CLIENT_SECRET"), "https://www.googleapis.com/auth/youtube.upload"}
	case "tiktok":
		return provider{"https://www.tiktok.com/v2/auth/authorize/", "https://open.tiktokapis.com/v2/oauth/token/", "client_key", os.Getenv("TIKTOK_CLIENT_KEY"), os.Getenv("TIKTOK_CLIENT_SECRET"), "user.info.basic,video.publish"}
	case "instagram":
		return provider{"https://www.instagram.com/oauth/authorize", "https://api.instagram.com/oauth/access_token", "client_id", os.Getenv("INSTAGRAM_CLIENT_ID"), os.Getenv("INSTAGRAM_CLIENT_SECRET"), "instagram_business_basic,instagram_business_content_publish"}
	}
	return provider{}
}
func callback(p string) string { return origin + "/api/oauth/" + p + "/callback" }
func configured(p string) bool {
	c := oauth(p)
	return c.id != "" && c.secret != "" && len(os.Getenv("TOKEN_SECRET")) >= 32 && (p == "youtube" || secure)
}
func crypt() (cipher.AEAD, error) {
	s := os.Getenv("TOKEN_SECRET")
	if len(s) < 32 {
		return nil, errors.New("TOKEN_SECRET minimal 32 karakter")
	}
	k := sha256.Sum256([]byte(s))
	block, e := aes.NewCipher(k[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func loadVault() error {
	vault.Tokens = map[string]token{}
	raw, e := os.ReadFile(filepath.Join(stateDir, "tokens.enc"))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	a, e := crypt()
	if e != nil {
		return e
	}
	if len(raw) < a.NonceSize() {
		return errors.New("token store rusak")
	}
	data, e := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], nil)
	if e != nil {
		return e
	}
	return json.Unmarshal(data, &vault.Tokens)
}
func saveVault() error {
	a, e := crypt()
	if e != nil {
		return e
	}
	data, e := json.Marshal(vault.Tokens)
	if e != nil {
		return e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	if e = os.MkdirAll(stateDir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(stateDir, "tokens-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(a.Seal(nonce, nonce, data, nil))
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(stateDir, "tokens.enc"))
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func request(ctx context.Context, method, endpoint, access, contentType string, body io.Reader, dst any) error {
	req, e := http.NewRequestWithContext(ctx, method, endpoint, body)
	if e != nil {
		return e
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("Koneksi ke platform gagal atau terputus")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Platform menolak permintaan (HTTP %d)", res.StatusCode)
	}
	if dst == nil {
		_, e = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return e
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 2<<20))
	decoder.UseNumber()
	return decoder.Decode(dst)
}
func exchange(ctx context.Context, p, code string, old token) (token, error) {
	c := oauth(p)
	args := url.Values{c.idKey: {c.id}, "client_secret": {c.secret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback(p)}}
	endpoint, method := c.exchange, "POST"
	if code == "" {
		args.Del("code")
		args.Del("redirect_uri")
		args.Set("grant_type", "refresh_token")
		args.Set("refresh_token", old.Refresh)
		if p == "instagram" {
			method = "GET"
			endpoint = "https://graph.instagram.com/refresh_access_token"
			args = url.Values{"grant_type": {"ig_refresh_token"}, "access_token": {old.Access}}
		}
	}
	var t token
	var e error
	if method == "GET" {
		e = request(ctx, method, endpoint+"?"+args.Encode(), "", "", nil, &t)
	} else {
		e = request(ctx, method, endpoint, "", "application/x-www-form-urlencoded", strings.NewReader(args.Encode()), &t)
	}
	if e != nil || t.Access == "" {
		return token{}, errors.New("Otorisasi gagal. Periksa izin aplikasi dan callback, lalu sambungkan ulang.")
	}
	if p == "instagram" && code != "" {
		var long token
		q := url.Values{"grant_type": {"ig_exchange_token"}, "client_secret": {c.secret}, "access_token": {t.Access}}
		if e = request(ctx, "GET", "https://graph.instagram.com/access_token?"+q.Encode(), "", "", nil, &long); e != nil || long.Access == "" {
			return token{}, errors.New("Gagal mendapatkan token Instagram jangka panjang")
		}
		long.UserID = t.UserID
		t = long
	}
	if t.Refresh == "" {
		t.Refresh = old.Refresh
	}
	if t.UserID == "" {
		t.UserID = old.UserID
	}
	t.Expires = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	return t, nil
}
func getToken(ctx context.Context, p string) (token, error) {
	vault.Lock()
	defer vault.Unlock()
	t := vault.Tokens[p]
	if t.Access == "" {
		return t, errors.New("Hubungkan akun terlebih dahulu")
	}
	if time.Until(t.Expires) > time.Minute {
		return t, nil
	}
	if p != "instagram" && t.Refresh == "" {
		return token{}, errors.New("Sambungkan ulang akun")
	}
	refreshed, e := exchange(ctx, p, "", t)
	if e != nil {
		return token{}, e
	}
	vault.Tokens[p] = refreshed
	return refreshed, saveVault()
}
func oauthStart(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("platform")
	if !configured(p) {
		apiError(w, 503, "Isi kredensial OAuth, TOKEN_SECRET dan APP_ORIGIN sesuai panduan")
		return
	}
	c := oauth(p)
	state := random()
	http.SetCookie(w, &http.Cookie{Name: "oauth_" + p, Value: state, Path: "/api/oauth/" + p, MaxAge: 600, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	q := url.Values{c.idKey: {c.id}, "response_type": {"code"}, "scope": {c.scope}, "state": {state}, "redirect_uri": {callback(p)}}
	if p == "youtube" {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	}
	if p == "instagram" {
		q.Set("enable_fb_login", "0")
		q.Set("force_authentication", "1")
	}
	http.Redirect(w, r, c.authorize+"?"+q.Encode(), http.StatusSeeOther)
}
func oauthCallback(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("platform")
	c, e := r.Cookie("oauth_" + p)
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	if e != nil || code == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 || !configured(p) {
		http.Redirect(w, r, "/?accounts=error", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_" + p, Path: "/api/oauth/" + p, MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	t, e := exchange(r.Context(), p, code, token{})
	if e == nil {
		vault.Lock()
		prior := vault.Tokens[p]
		vault.Tokens[p] = t
		if e = saveVault(); e != nil {
			vault.Tokens[p] = prior
		}
		vault.Unlock()
	}
	result := "connected"
	if e != nil {
		result = "error"
		log.Print(p, ": ", e)
	}
	http.Redirect(w, r, "/?accounts="+result, http.StatusSeeOther)
}
func accounts(w http.ResponseWriter, r *http.Request) {
	vault.Lock()
	defer vault.Unlock()
	statuses := map[string]any{}
	for _, p := range platformNames {
		t := vault.Tokens[p]
		statuses[p] = map[string]any{"configured": configured(p), "connected": configured(p) && t.Access != "" && (time.Now().Before(t.Expires) || t.Refresh != ""), "callback": callback(p)}
	}
	jsonOut(w, 200, statuses)
}
func disconnect(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("platform")
	vault.Lock()
	defer vault.Unlock()
	old, ok := vault.Tokens[p]
	if !ok {
		w.WriteHeader(204)
		return
	}
	delete(vault.Tokens, p)
	if e := saveVault(); e != nil {
		vault.Tokens[p] = old
		apiError(w, 500, "Gagal menyimpan perubahan")
		return
	}
	w.WriteHeader(204)
}
func postJSON(ctx context.Context, endpoint, access string, payload, dst any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	return request(ctx, "POST", endpoint, access, "application/json", bytes.NewReader(b), dst)
}

func same(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
