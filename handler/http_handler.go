package handler

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"linkdesk/model"
	"linkdesk/service"
)

// Define a custom key type to avoid collisions in context values
type contextKey string

// Define context keys
const currentYearKey contextKey = "currentYear"

// HTTPHandler handles HTTP requests
type HTTPHandler struct {
	urlService service.URLServiceInterface
	baseURL    string
	templates  *template.Template
}

// NewHTTPHandler creates a new HTTP handler
func NewHTTPHandler(urlService service.URLServiceInterface, baseURL string, templatesDir string) (*HTTPHandler, error) {
	templates := template.New("")

	templates = templates.Funcs(template.FuncMap{
		"expired": func(u *model.URL) bool { return u.Expired(time.Now()) },
		"formatTime": func(t time.Time) string {
			return t.UTC().Format("2006-01-02 15:04 UTC")
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
	})

	templates, err := templates.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &HTTPHandler{
		urlService: urlService,
		baseURL:    baseURL,
		templates:  templates,
	}, nil
}

// SetupRoutes sets up the HTTP routes. If both username and password are non-empty,
// all management routes are protected with HTTP Basic Auth. The redirect endpoint
// (/{code}) is always public so that short links work without authentication.
func (h *HTTPHandler) SetupRoutes(router chi.Router, username, password string) {
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(h.currentYearMiddleware)
	router.Use(http.NewCrossOriginProtection().Handler)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })

	// Always public: static assets, favicon, and the redirect itself
	fileServer := http.FileServer(http.Dir("./static"))
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))
	router.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/favicon.ico")
	})
	router.Get("/{code}", h.redirectHandler)

	// Management routes, optionally protected
	router.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				r.Body = http.MaxBytesReader(w, r.Body, 16384)
				next.ServeHTTP(w, r)
			})
		})
		if username != "" && password != "" {
			r.Use(basicAuthMiddleware(username, password))
		}

		r.Get("/", h.indexHandler)
		r.Get("/urls", h.listURLsHandler)
		r.Get("/manage/{code}", h.resultHandler)
		r.Post("/shorten", h.shortenURLHandler)
		r.Get("/qr/{code}", h.qrCodeHandler)
		r.Post("/delete/{code}", h.deleteURLHandler)

		r.Route("/api", func(r chi.Router) {
			r.Post("/shorten", h.apiShortenURLHandler)
			r.Get("/urls", h.apiListURLsHandler)
			r.Get("/url/{code}", h.apiGetURLHandler)
			r.Delete("/url/{code}", h.apiDeleteURLHandler)
		})
	})
}

// basicAuthMiddleware returns an HTTP Basic Auth middleware with the given credentials
func basicAuthMiddleware(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="URL Shortener"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// currentYearMiddleware adds the current year to the request context
func (h *HTTPHandler) currentYearMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, currentYearKey, time.Now().Format("2006"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// renderError writes an error page with the given status code and message
func (h *HTTPHandler) renderError(w http.ResponseWriter, r *http.Request, statusCode int, msg string) {
	h.render(w, statusCode, map[string]any{
		"error":       msg,
		"currentYear": r.Context().Value(currentYearKey),
	})
}

func (h *HTTPHandler) render(w http.ResponseWriter, status int, data map[string]any) {
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, "base.html", data); err != nil {
		log.Printf("render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := body.WriteTo(w); err != nil {
		log.Printf("write page: %v", err)
	}
}

// indexHandler handles the index page
func (h *HTTPHandler) indexHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "base.html", map[string]any{
		"baseURL":     h.baseURL,
		"currentYear": r.Context().Value(currentYearKey),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
	}
}

// listURLsHandler handles the URL listing page
func (h *HTTPHandler) listURLsHandler(w http.ResponseWriter, r *http.Request) {
	opt, err := parseListOptions(r)
	if err != nil {
		h.renderError(w, r, 400, err.Error())
		return
	}
	page, err := h.urlService.SearchURLs(r.Context(), opt)
	if err != nil {
		var invalid *model.ErrInvalidInput
		if errors.As(err, &invalid) {
			h.renderError(w, r, 400, err.Error())
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load URLs")
		return
	}
	prev, next := "", ""
	if opt.Page > 1 {
		prev = pageLink(opt, opt.Page-1)
	}
	if opt.Page < page.TotalPages {
		next = pageLink(opt, opt.Page+1)
	}
	h.render(w, http.StatusOK, map[string]any{
		"isList": true, "urls": page.URLs, "page": page,
		"query": opt.Query, "status": opt.Status, "prev": prev, "next": next,
		"baseURL":     h.baseURL,
		"currentYear": r.Context().Value(currentYearKey),
	})
}

// shortenURLHandler handles URL shortening requests
func (h *HTTPHandler) shortenURLHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	longURL := r.PostForm.Get("url")
	customCode := r.PostForm.Get("custom_code")
	var expiry *time.Time
	if raw := r.PostForm.Get("ttl_hours"); raw != "" && raw != "0" {
		hours, err := strconv.Atoi(raw)
		if err != nil || hours < 1 || hours > 8760 {
			h.renderError(w, r, 400, "有效期须为 1～8760 小时")
			return
		}
		t := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
		expiry = &t
	}

	if longURL == "" {
		h.renderError(w, r, http.StatusBadRequest, "URL is required")
		return
	}

	url, err := h.urlService.ShortenURL(longURL, customCode, expiry)
	if err != nil {
		statusCode, msg := classifyServiceError(err)
		h.renderError(w, r, statusCode, msg)
		return
	}

	// Refreshing the result page must not repeat the INSERT.
	http.Redirect(w, r, "/manage/"+url.ShortCode, http.StatusSeeOther)
}

func (h *HTTPHandler) resultHandler(w http.ResponseWriter, r *http.Request) {
	u, err := h.urlService.GetURL(chi.URLParam(r, "code"))
	if err != nil {
		h.renderError(w, r, 500, "Failed to load link")
		return
	}
	if u == nil {
		h.renderError(w, r, 404, "链接不存在或已删除。")
		return
	}
	h.render(w, 200, map[string]any{"url": u, "shortURL": h.baseURL + "/" + u.ShortCode, "baseURL": h.baseURL, "currentYear": r.Context().Value(currentYearKey)})
}

// qrCodeHandler generates a QR code for a URL
func (h *HTTPHandler) qrCodeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	shortURL := fmt.Sprintf("%s/%s", h.baseURL, code)

	qrCode, err := h.urlService.GenerateQRCode(shortURL)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to generate QR code")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	if _, err = w.Write(qrCode); err != nil {
		log.Printf("failed to write QR code response: %v", err)
	}
}

// deleteURLHandler handles URL deletion requests
func (h *HTTPHandler) deleteURLHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	if err := h.urlService.DeleteURL(code); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to delete URL")
		return
	}

	http.Redirect(w, r, "/urls", http.StatusFound)
}

// redirectHandler redirects to the original URL
func (h *HTTPHandler) redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	url, err := h.urlService.GetURL(code)
	if err != nil {
		http.Error(w, "Failed to retrieve URL", http.StatusInternalServerError)
		return
	}

	if url == nil {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	if url.Expired(time.Now()) {
		h.renderError(w, r, http.StatusGone, "链接已到期，请向分享者申请新的链接。")
		return
	}
	// Synchronous update makes counts observable immediately and avoids unbounded goroutines.
	if err := h.urlService.RecordClick(code); err != nil {
		log.Printf("record click for %s: %v", code, err)
	}

	http.Redirect(w, r, url.LongURL, http.StatusFound)
}

// API Handlers

// apiShortenURLHandler handles API URL shortening requests
func (h *HTTPHandler) apiShortenURLHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL        string     `json:"url"`
		CustomCode string     `json:"custom_code"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", 415)
		return
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Expected one JSON object", 400)
		return
	}

	if request.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	url, err := h.urlService.ShortenURL(request.URL, request.CustomCode, request.ExpiresAt)
	if err != nil {
		statusCode, msg := classifyServiceError(err)
		http.Error(w, msg, statusCode)
		return
	}

	shortURL := fmt.Sprintf("%s/%s", h.baseURL, url.ShortCode)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":         url.ID,
		"short_code": url.ShortCode,
		"long_url":   url.LongURL,
		"short_url":  shortURL,
		"created_at": url.CreatedAt.Format(time.RFC3339),
		"clicks":     url.Clicks,
		"expires_at": url.ExpiresAt,
	})
}

// apiListURLsHandler handles API URL listing requests
func (h *HTTPHandler) apiListURLsHandler(w http.ResponseWriter, r *http.Request) {
	opt, err := parseListOptions(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	page, err := h.urlService.SearchURLs(r.Context(), opt)
	if err != nil {
		var invalid *model.ErrInvalidInput
		if errors.As(err, &invalid) {
			http.Error(w, err.Error(), 400)
			return
		}
		http.Error(w, "Failed to list URLs", http.StatusInternalServerError)
		return
	}

	response := make([]map[string]any, 0, len(page.URLs))
	for _, url := range page.URLs {
		shortURL := fmt.Sprintf("%s/%s", h.baseURL, url.ShortCode)
		response = append(response, map[string]any{
			"id":         url.ID,
			"short_code": url.ShortCode,
			"long_url":   url.LongURL,
			"short_url":  shortURL,
			"created_at": url.CreatedAt.Format(time.RFC3339),
			"clicks":     url.Clicks,
			"expires_at": url.ExpiresAt,
			"expired":    url.Expired(time.Now()),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{"urls": response, "total": page.Total, "page": page.Page, "page_size": page.PageSize, "total_pages": page.TotalPages})
}

// apiGetURLHandler handles API URL retrieval requests
func (h *HTTPHandler) apiGetURLHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	url, err := h.urlService.GetURL(code)
	if err != nil {
		http.Error(w, "Failed to retrieve URL", http.StatusInternalServerError)
		return
	}

	if url == nil {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	shortURL := fmt.Sprintf("%s/%s", h.baseURL, url.ShortCode)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":         url.ID,
		"short_code": url.ShortCode,
		"long_url":   url.LongURL,
		"short_url":  shortURL,
		"created_at": url.CreatedAt.Format(time.RFC3339),
		"clicks":     url.Clicks,
		"expires_at": url.ExpiresAt,
		"expired":    url.Expired(time.Now()),
	})
}

// apiDeleteURLHandler handles API URL deletion requests
func (h *HTTPHandler) apiDeleteURLHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	if err := h.urlService.DeleteURL(code); err != nil {
		http.Error(w, "Failed to delete URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "URL deleted successfully"})
}

// classifyServiceError maps service errors to HTTP status codes and safe messages
func classifyServiceError(err error) (int, string) {
	var errInvalid *model.ErrInvalidURL
	var errInvalidCode *model.ErrInvalidCode
	var errCodeInUse *model.ErrCustomCodeAlreadyExists
	var errInput *model.ErrInvalidInput

	switch {
	case errors.As(err, &errInvalid), errors.As(err, &errInvalidCode), errors.As(err, &errInput):
		return http.StatusBadRequest, err.Error()
	case errors.As(err, &errCodeInUse):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "Failed to shorten URL"
	}
}
