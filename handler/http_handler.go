package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mstgnz/self-hosted-url-shortener/model"
	"github.com/mstgnz/self-hosted-url-shortener/service"
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
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
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

	// Always public: static assets, favicon, and the redirect itself
	fileServer := http.FileServer(http.Dir("./static"))
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))
	router.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/favicon.ico")
	})
	router.Get("/{code}", h.redirectHandler)

	// Management routes, optionally protected
	router.Group(func(r chi.Router) {
		if username != "" && password != "" {
			r.Use(basicAuthMiddleware(username, password))
		}

		r.Get("/", h.indexHandler)
		r.Get("/urls", h.listURLsHandler)
		r.Post("/shorten", h.shortenURLHandler)
		r.Get("/qr/{code}", h.qrCodeHandler)
		r.Get("/delete/{code}", h.deleteURLHandler)

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
			if !ok || u != username || p != password {
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
	w.WriteHeader(statusCode)
	if err := h.templates.ExecuteTemplate(w, "base.html", map[string]any{
		"error":       msg,
		"currentYear": r.Context().Value(currentYearKey),
	}); err != nil {
		log.Printf("failed to execute error template: %v", err)
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
	urls, err := h.urlService.ListURLs()
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load URLs")
		return
	}

	if err = h.templates.ExecuteTemplate(w, "base.html", map[string]any{
		"urls":        urls,
		"baseURL":     h.baseURL,
		"currentYear": r.Context().Value(currentYearKey),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
	}
}

// shortenURLHandler handles URL shortening requests
func (h *HTTPHandler) shortenURLHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	longURL := r.PostForm.Get("url")
	customCode := r.PostForm.Get("custom_code")

	if longURL == "" {
		h.renderError(w, r, http.StatusBadRequest, "URL is required")
		return
	}

	url, err := h.urlService.ShortenURL(longURL, customCode)
	if err != nil {
		statusCode, msg := classifyServiceError(err)
		h.renderError(w, r, statusCode, msg)
		return
	}

	shortURL := fmt.Sprintf("%s/%s", h.baseURL, url.ShortCode)

	if err = h.templates.ExecuteTemplate(w, "base.html", map[string]any{
		"url":         url,
		"shortURL":    shortURL,
		"baseURL":     h.baseURL,
		"currentYear": r.Context().Value(currentYearKey),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template: %v", err), http.StatusInternalServerError)
	}
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

	go func() {
		if err := h.urlService.RecordClick(code); err != nil {
			var errNotFound *model.ErrURLNotFound
			if !errors.As(err, &errNotFound) {
				log.Printf("failed to record click for %s: %v", code, err)
			}
		}
	}()

	http.Redirect(w, r, url.LongURL, http.StatusFound)
}

// API Handlers

// apiShortenURLHandler handles API URL shortening requests
func (h *HTTPHandler) apiShortenURLHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL        string `json:"url"`
		CustomCode string `json:"custom_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	url, err := h.urlService.ShortenURL(request.URL, request.CustomCode)
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
	})
}

// apiListURLsHandler handles API URL listing requests
func (h *HTTPHandler) apiListURLsHandler(w http.ResponseWriter, r *http.Request) {
	urls, err := h.urlService.ListURLs()
	if err != nil {
		http.Error(w, "Failed to list URLs", http.StatusInternalServerError)
		return
	}

	var response []map[string]any
	for _, url := range urls {
		shortURL := fmt.Sprintf("%s/%s", h.baseURL, url.ShortCode)
		response = append(response, map[string]any{
			"id":         url.ID,
			"short_code": url.ShortCode,
			"long_url":   url.LongURL,
			"short_url":  shortURL,
			"created_at": url.CreatedAt.Format(time.RFC3339),
			"clicks":     url.Clicks,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"urls": response})
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

	switch {
	case errors.As(err, &errInvalid), errors.As(err, &errInvalidCode):
		return http.StatusBadRequest, err.Error()
	case errors.As(err, &errCodeInUse):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "Failed to shorten URL"
	}
}
