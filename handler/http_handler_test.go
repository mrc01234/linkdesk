package handler

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"linkdesk/model"
	"linkdesk/service"
)

// MockURLService is a mock implementation of the URL service for testing
type MockURLService struct {
	urls      map[string]*model.URL
	id        int64
	listErr   error
	deleteErr error
	getErr    error
}

// NewMockURLService creates a new mock URL service
func NewMockURLService() *MockURLService {
	return &MockURLService{
		urls: make(map[string]*model.URL),
		id:   1,
	}
}

func (m *MockURLService) ShortenURL(longURL, customCode string, expiry ...*time.Time) (*model.URL, error) {
	shortCode := customCode
	if shortCode == "" {
		shortCode = "generated"
	}

	if _, exists := m.urls[shortCode]; exists && customCode != "" {
		return nil, &model.ErrCustomCodeAlreadyExists{Code: customCode}
	}

	u := &model.URL{
		ID:        m.id,
		ShortCode: shortCode,
		LongURL:   longURL,
		CreatedAt: time.Now(),
		Clicks:    0,
	}
	m.id++
	if len(expiry) > 0 {
		u.ExpiresAt = expiry[0]
	}
	m.urls[shortCode] = u
	return u, nil
}

func (m *MockURLService) GetURL(shortCode string) (*model.URL, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	u, exists := m.urls[shortCode]
	if !exists {
		return nil, nil
	}
	return u, nil
}

func (m *MockURLService) RecordClick(shortCode string) error {
	u, exists := m.urls[shortCode]
	if !exists {
		return &model.ErrURLNotFound{Code: shortCode}
	}
	u.Clicks++
	return nil
}

func (m *MockURLService) ListURLs() ([]*model.URL, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	urls := make([]*model.URL, 0, len(m.urls))
	for _, u := range m.urls {
		urls = append(urls, u)
	}
	return urls, nil
}

func (m *MockURLService) SearchURLs(ctx context.Context, opt model.ListOptions) (*model.URLPage, error) {
	urls, err := m.ListURLs()
	if err != nil {
		return nil, err
	}
	return &model.URLPage{URLs: urls, Total: len(urls), Page: opt.Page, PageSize: opt.PageSize}, nil
}

func (m *MockURLService) DeleteURL(shortCode string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.urls, shortCode)
	return nil
}

func (m *MockURLService) GenerateQRCode(shortURL string) ([]byte, error) {
	return []byte("mock-qr-code"), nil
}

// Ensure MockURLService implements service.URLService interface
var _ service.URLServiceInterface = (*MockURLService)(nil)

const mockTemplateText = `
{{ define "base.html" }}ok{{ end }}
{{ define "error-content" }}error: {{ .error }}{{ end }}
`

func setupTestHandler(t *testing.T) (*HTTPHandler, *MockURLService) {
	t.Helper()
	mockService := NewMockURLService()

	tmpl, err := template.New("main").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string { return t.Format("2006-01-02") },
		"truncate":   func(s string, n int) string { return s },
	}).Parse(mockTemplateText)
	if err != nil {
		t.Fatalf("Failed to create mock template: %v", err)
	}

	h := &HTTPHandler{
		urlService: mockService,
		baseURL:    "http://localhost:8080",
		templates:  tmpl,
	}

	return h, mockService
}

func withYear(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentYearKey, "2024"))
}

func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestIndexHandler(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withYear(req)
	w := httptest.NewRecorder()

	h.indexHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestListURLsHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "test1")
		svc.ShortenURL("https://example.org", "test2")

		req := httptest.NewRequest(http.MethodGet, "/urls", nil)
		req = withYear(req)
		w := httptest.NewRecorder()

		h.listURLsHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("ServiceError", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.listErr = errors.New("db down")

		req := httptest.NewRequest(http.MethodGet, "/urls", nil)
		req = withYear(req)
		w := httptest.NewRecorder()

		h.listURLsHandler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})
}

func TestShortenURLHandler(t *testing.T) {
	t.Run("ValidURL", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		form := url.Values{}
		form.Set("url", "https://example.com")
		form.Set("custom_code", "mycode")
		req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withYear(req)
		w := httptest.NewRecorder()

		h.shortenURLHandler(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("Expected %d, got %d", http.StatusSeeOther, w.Code)
		}
	})

	t.Run("MissingURL", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		form := url.Values{}
		req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withYear(req)
		w := httptest.NewRecorder()

		h.shortenURLHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("DuplicateCustomCode", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "taken")

		form := url.Values{}
		form.Set("url", "https://other.com")
		form.Set("custom_code", "taken")
		req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withYear(req)
		w := httptest.NewRecorder()

		h.shortenURLHandler(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected %d, got %d", http.StatusConflict, w.Code)
		}
	})
}

func TestDeleteURLHandler(t *testing.T) {
	t.Run("ExistingCode", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "todelete")

		req := httptest.NewRequest(http.MethodGet, "/delete/todelete", nil)
		req = withChiParam(req, "code", "todelete")
		w := httptest.NewRecorder()

		h.deleteURLHandler(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("Expected %d, got %d", http.StatusFound, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/urls" {
			t.Errorf("Expected redirect to '/urls', got '%s'", loc)
		}
	})

	t.Run("ServiceError", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.deleteErr = errors.New("db error")

		req := httptest.NewRequest(http.MethodGet, "/delete/x", nil)
		req = withChiParam(withYear(req), "code", "x")
		w := httptest.NewRecorder()

		h.deleteURLHandler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})
}

func TestRedirectHandler(t *testing.T) {
	t.Run("ExistingCode", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "test")

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = withChiParam(req, "code", "test")
		w := httptest.NewRecorder()

		h.redirectHandler(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusFound {
			t.Errorf("Expected %d, got %d", http.StatusFound, resp.StatusCode)
		}
		if resp.Header.Get("Location") != "https://example.com" {
			t.Errorf("Expected redirect to 'https://example.com', got '%s'", resp.Header.Get("Location"))
		}
	})

	t.Run("NonExistentCode", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req = withChiParam(req, "code", "missing")
		w := httptest.NewRecorder()

		h.redirectHandler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("ServiceError", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.getErr = errors.New("db error")

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req = withChiParam(req, "code", "x")
		w := httptest.NewRecorder()

		h.redirectHandler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})
}

func TestQRCodeHandler(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/qr/test", nil)
	req = withChiParam(req, "code", "test")
	w := httptest.NewRecorder()

	h.qrCodeHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Expected Content-Type 'image/png', got '%s'", ct)
	}
}

func TestAPIHandlers(t *testing.T) {
	t.Run("APIShortenURL", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		body := `{"url": "https://example.com", "custom_code": "api-test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.apiShortenURLHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
		}
		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp["short_code"] != "api-test" {
			t.Errorf("Expected short_code 'api-test', got '%v'", resp["short_code"])
		}
		if resp["long_url"] != "https://example.com" {
			t.Errorf("Expected long_url 'https://example.com', got '%v'", resp["long_url"])
		}
	})

	t.Run("APIShortenURL_MissingURL", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.apiShortenURLHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("APIShortenURL_DuplicateCode", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "taken")

		body := `{"url": "https://other.com", "custom_code": "taken"}`
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.apiShortenURLHandler(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected %d, got %d", http.StatusConflict, w.Code)
		}
	})

	t.Run("APIListURLs", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/urls", nil)
		w := httptest.NewRecorder()

		h.apiListURLsHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("APIListURLs_ServiceError", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.listErr = errors.New("db down")

		req := httptest.NewRequest(http.MethodGet, "/api/urls", nil)
		w := httptest.NewRecorder()

		h.apiListURLsHandler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("APIGetURL_Found", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "api-test")

		req := httptest.NewRequest(http.MethodGet, "/api/url/api-test", nil)
		req = withChiParam(req, "code", "api-test")
		w := httptest.NewRecorder()

		h.apiGetURLHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
		}
		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp["short_code"] != "api-test" {
			t.Errorf("Expected short_code 'api-test', got '%v'", resp["short_code"])
		}
	})

	t.Run("APIGetURL_NotFound", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/url/missing", nil)
		req = withChiParam(req, "code", "missing")
		w := httptest.NewRecorder()

		h.apiGetURLHandler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("APIDeleteURL", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.ShortenURL("https://example.com", "api-test")

		req := httptest.NewRequest(http.MethodDelete, "/api/url/api-test", nil)
		req = withChiParam(req, "code", "api-test")
		w := httptest.NewRecorder()

		h.apiDeleteURLHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("APIDeleteURL_ServiceError", func(t *testing.T) {
		h, svc := setupTestHandler(t)
		svc.deleteErr = errors.New("db error")

		req := httptest.NewRequest(http.MethodDelete, "/api/url/x", nil)
		req = withChiParam(req, "code", "x")
		w := httptest.NewRecorder()

		h.apiDeleteURLHandler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	t.Run("APIShortenURL_InvalidBody", func(t *testing.T) {
		h, _ := setupTestHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.apiShortenURLHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestClassifyServiceError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"InvalidURL", &model.ErrInvalidURL{URL: "bad"}, http.StatusBadRequest},
		{"InvalidCode", &model.ErrInvalidCode{Code: "x", Reason: "reserved"}, http.StatusBadRequest},
		{"CodeInUse", &model.ErrCustomCodeAlreadyExists{Code: "taken"}, http.StatusConflict},
		{"UnknownError", errors.New("some db error"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := classifyServiceError(tc.err)
			if status != tc.wantStatus {
				t.Errorf("Expected %d, got %d", tc.wantStatus, status)
			}
		})
	}
}
