package httpapi

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/farstar-team/panel/internal/config"
	"github.com/farstar-team/panel/internal/engine"
	"github.com/farstar-team/panel/internal/manager"
	"github.com/farstar-team/panel/internal/security"
	"github.com/farstar-team/panel/internal/store"
	"github.com/farstar-team/panel/internal/systeminfo"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	store   *store.Store
	vault   *security.Vault
	manager *manager.Manager
	config  config.Config
	mux     *http.ServeMux
	limiter *loginLimiter
}

type sessionContext struct {
	UserID int64
	CSRF   string
	Token  string
}

type contextKey string

const sessionKey contextKey = "session"

func New(s *store.Store, vault *security.Vault, mgr *manager.Manager, cfg config.Config) *Server {
	server := &Server{
		store: s, vault: vault, manager: mgr, config: cfg,
		mux: http.NewServeMux(), limiter: newLoginLimiter(),
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.requestLog(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.Handle("POST /api/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	s.mux.Handle("GET /api/session", s.requireAuth(http.HandlerFunc(s.session)))
	s.mux.Handle("GET /api/tunnels", s.requireAuth(http.HandlerFunc(s.listTunnels)))
	s.mux.Handle("POST /api/tunnels", s.requireAuth(http.HandlerFunc(s.createTunnel)))
	s.mux.Handle("GET /api/tunnels/{id}", s.requireAuth(http.HandlerFunc(s.getTunnel)))
	s.mux.Handle("PUT /api/tunnels/{id}", s.requireAuth(http.HandlerFunc(s.updateTunnel)))
	s.mux.Handle("DELETE /api/tunnels/{id}", s.requireAuth(http.HandlerFunc(s.deleteTunnel)))
	s.mux.Handle("POST /api/tunnels/{id}/{action}", s.requireAuth(http.HandlerFunc(s.tunnelAction)))
	s.mux.Handle("GET /api/tunnels/{id}/logs", s.requireAuth(http.HandlerFunc(s.logs)))
	s.mux.Handle("GET /api/system", s.requireAuth(http.HandlerFunc(s.system)))
	s.mux.Handle("GET /api/events", s.requireAuth(http.HandlerFunc(s.events)))
	s.mux.Handle("GET /api/backup", s.requireAuth(http.HandlerFunc(s.backup)))
	s.mux.HandleFunc("/", s.static)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "تعداد تلاش‌ها زیاد است؛ چند دقیقه بعد دوباره امتحان کنید.")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "درخواست نامعتبر است.")
		return
	}
	userID, hash, err := s.store.UserByName(r.Context(), strings.TrimSpace(input.Username))
	if err != nil || !security.VerifyPassword(hash, input.Password) {
		s.limiter.Failed(ip)
		time.Sleep(350 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "نام کاربری یا رمز عبور صحیح نیست.")
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ساخت نشست ناموفق بود.")
		return
	}
	csrf, _ := security.RandomToken(24)
	if err := s.store.CreateSession(r.Context(), tokenHash(token), userID, csrf, time.Now().Add(24*time.Hour)); err != nil {
		writeError(w, http.StatusInternalServerError, "ذخیره نشست ناموفق بود.")
		return
	}
	s.limiter.Success(ip)
	http.SetCookie(w, &http.Cookie{
		Name: "farstar_session", Value: token, Path: "/", MaxAge: 86400,
		HttpOnly: true, Secure: s.config.CookieSecure || r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrf": csrf})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	session := sessionFrom(r.Context())
	_ = s.store.DeleteSession(r.Context(), tokenHash(session.Token))
	http.SetCookie(w, &http.Cookie{
		Name: "farstar_session", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.config.CookieSecure || r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	session := sessionFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "csrf": session.CSRF})
}

func (s *Server) listTunnels(w http.ResponseWriter, r *http.Request) {
	tunnels, err := s.store.ListTunnels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "خواندن تانل‌ها ناموفق بود.")
		return
	}
	if tunnels == nil {
		tunnels = []store.Tunnel{}
	}
	writeJSON(w, http.StatusOK, tunnels)
}

func (s *Server) getTunnel(w http.ResponseWriter, r *http.Request) {
	tunnel, err := s.store.Tunnel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "تانل پیدا نشد.")
		return
	}
	writeJSON(w, http.StatusOK, tunnel)
}

func (s *Server) createTunnel(w http.ResponseWriter, r *http.Request) {
	var tunnel store.Tunnel
	if err := decodeJSON(r, &tunnel); err != nil {
		writeError(w, http.StatusBadRequest, "اطلاعات تانل نامعتبر است.")
		return
	}
	tunnel.Name = strings.TrimSpace(tunnel.Name)
	tunnel.ID, _ = security.RandomToken(12)
	if tunnel.Secret == "" {
		tunnel.Secret, _ = security.RandomToken(24)
	}
	if err := engine.Validate(tunnel, tunnel.Secret); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ciphertext, err := s.vault.Encrypt(tunnel.Secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "رمزگذاری راز تانل ناموفق بود.")
		return
	}
	tunnel.SecretCipher = ciphertext
	if err := s.store.CreateTunnel(r.Context(), tunnel); err != nil {
		writeError(w, http.StatusConflict, friendlyDBError(err))
		return
	}
	tunnel.Secret = ""
	writeJSON(w, http.StatusCreated, tunnel)
}

func (s *Server) updateTunnel(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.Tunnel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "تانل پیدا نشد.")
		return
	}
	if current.Status == "running" || current.Status == "starting" {
		writeError(w, http.StatusConflict, "برای ویرایش، ابتدا تانل را متوقف کنید.")
		return
	}
	var input store.Tunnel
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "اطلاعات تانل نامعتبر است.")
		return
	}
	input.ID = current.ID
	input.SecretCipher = current.SecretCipher
	secret, err := s.vault.Decrypt(current.SecretCipher)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "خواندن راز تانل ناموفق بود.")
		return
	}
	if input.Secret != "" {
		secret = input.Secret
		input.SecretCipher, err = s.vault.Encrypt(secret)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "رمزگذاری راز تانل ناموفق بود.")
			return
		}
	}
	if err := engine.Validate(input, secret); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateTunnel(r.Context(), input); err != nil {
		writeError(w, http.StatusConflict, friendlyDBError(err))
		return
	}
	input.Secret = ""
	writeJSON(w, http.StatusOK, input)
}

func (s *Server) deleteTunnel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteTunnel(r.Context(), id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	_ = os.Remove(s.manager.LogPath(id))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tunnelAction(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.PathValue("action") {
	case "start":
		err = s.manager.Start(r.Context(), r.PathValue("id"))
	case "stop":
		err = s.manager.Stop(r.Context(), r.PathValue("id"))
	case "restart":
		err = s.manager.Restart(r.Context(), r.PathValue("id"))
	default:
		writeError(w, http.StatusNotFound, "عملیات ناشناخته است.")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(s.manager.LogPath(r.PathValue("id")))
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]string{"logs": ""})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "خواندن لاگ ناموفق بود.")
		return
	}
	defer file.Close()
	info, _ := file.Stat()
	const maxBytes int64 = 256 * 1024
	if info != nil && info.Size() > maxBytes {
		_, _ = file.Seek(-maxBytes, io.SeekEnd)
	}
	data, _ := io.ReadAll(io.LimitReader(file, maxBytes))
	writeJSON(w, http.StatusOK, map[string]string{"logs": string(data)})
}

func (s *Server) system(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, systeminfo.Read())
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "مرورگر از رویداد زنده پشتیبانی نمی‌کند.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		tunnels, err := s.store.ListTunnels(r.Context())
		if err == nil {
			payload, _ := json.Marshal(map[string]any{
				"tunnels": tunnels,
				"system":  systeminfo.Read(),
				"time":    time.Now().Unix(),
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	tunnels, err := s.store.ListTunnels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ساخت پشتیبان ناموفق بود.")
		return
	}
	for i := range tunnels {
		tunnels[i].Secret, _ = s.vault.Decrypt(tunnels[i].SecretCipher)
		tunnels[i].PID = 0
		tunnels[i].Status = "stopped"
	}
	payload := struct {
		Product string         `json:"product"`
		Version int            `json:"version"`
		Date    string         `json:"date"`
		Tunnels []store.Tunnel `json:"tunnels"`
	}{"Farstar Tunnel Panel", 1, time.Now().UTC().Format(time.RFC3339), tunnels}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="farstar-backup.json"`)
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("farstar_session")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "نشست معتبر نیست.")
			return
		}
		userID, csrf, err := s.store.Session(r.Context(), tokenHash(cookie.Value))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "نشست منقضی شده است.")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			r.Header.Get("X-CSRF-Token") != csrf {
			writeError(w, http.StatusForbidden, "توکن امنیتی درخواست معتبر نیست.")
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey, sessionContext{
			UserID: userID, CSRF: csrf, Token: cookie.Value,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "مسیر API پیدا نشد.")
		return
	}
	clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." {
		clean = "index.html"
	}
	sub, _ := fs.Sub(webFiles, "web")
	data, err := fs.ReadFile(sub, clean)
	if err != nil {
		data, err = fs.ReadFile(sub, "index.html")
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(clean)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/api/events") {
			log.Printf("%s %s %s %s", clientIP(r), r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, (1<<20)+1))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func sessionFrom(ctx context.Context) sessionContext {
	value, _ := ctx.Value(sessionKey).(sessionContext)
	return value
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func friendlyDBError(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") {
		return "نام تانل تکراری است."
	}
	return "ذخیره اطلاعات ناموفق بود."
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string][]time.Time)}
}

func (l *loginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Minute)
	var recent []time.Time
	for _, attempt := range l.attempts[ip] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	l.attempts[ip] = recent
	return len(recent) < 5
}

func (l *loginLimiter) Failed(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[ip] = append(l.attempts[ip], time.Now())
}

func (l *loginLimiter) Success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

func ParseLimit(value string, fallback int) int {
	result, err := strconv.Atoi(value)
	if err != nil || result <= 0 {
		return fallback
	}
	return result
}
