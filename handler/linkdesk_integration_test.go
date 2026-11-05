package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"linkdesk/database"
	"linkdesk/model"
	"linkdesk/service"
)

func realApp(t *testing.T, authenticated bool) (http.Handler, *database.Database) {
	t.Helper()
	d, err := database.New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	h, err := NewHTTPHandler(service.New(d), "http://example.com", "../templates")
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	if authenticated {
		h.SetupRoutes(r, "admin", "test-password")
	} else {
		h.SetupRoutes(r, "", "")
	}
	return r, d
}
func call(app http.Handler, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if auth {
		r.SetBasicAuth("admin", "test-password")
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}
func TestRealPagesEmptyAndPagination(t *testing.T) {
	app, d := realApp(t, false)
	w := call(app, "GET", "/urls", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "没有匹配的链接") || strings.Contains(w.Body.String(), "<section class=\"intro\">") {
		t.Fatalf("empty page %d: %s", w.Code, w.Body)
	}
	w = call(app, "GET", "/api/urls", "", false)
	var p struct {
		URLs  []map[string]any `json:"urls"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.URLs == nil || p.Total != 0 || p.Page != 1 {
		t.Fatalf("empty JSON: %s %v", w.Body, err)
	}
	for i := 0; i < 12; i++ {
		if err := d.SaveURL(model.NewURL(fmt.Sprintf("doc-%02d", i), "https://example.com/guide")); err != nil {
			t.Fatal(err)
		}
	}
	w = call(app, "GET", "/api/urls?q=doc&page=2&page_size=5", "", false)
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.Total != 12 || len(p.URLs) != 5 || p.Page != 2 {
		t.Fatalf("pagination: %s %v", w.Body, err)
	}
	w = call(app, "GET", "/urls?q=doc&page_size=5", "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "下一页") || !strings.Contains(w.Body.String(), "page=2") {
		t.Fatalf("real template: %d %s", w.Code, w.Body)
	}
	for _, query := range []string{"page=0", "page=-1", "page=x", "page=", "page=1000001", "page=1&page=2", "page_size=101", "page_size=-3", "status=oops", "q=" + strings.Repeat("x", 201)} {
		w = call(app, "GET", "/api/urls?"+query, "", false)
		if w.Code != 400 {
			t.Errorf("%s => %d", query, w.Code)
		}
	}
}

func TestRealCreateExpiryRedirectAndAuth(t *testing.T) {
	app, d := realApp(t, true)
	if w := call(app, "GET", "/api/urls", "", false); w.Code != 401 {
		t.Fatalf("auth=%d", w.Code)
	}
	if w := call(app, "GET", "/healthz", "", false); w.Code != 200 {
		t.Fatalf("health=%d", w.Code)
	}
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	body := fmt.Sprintf(`{"url":"https://example.org/guide","custom_code":"guide","expires_at":%q}`, future.Format(time.RFC3339))
	w := call(app, "POST", "/api/shorten", body, true)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("expires_at")) {
		t.Fatalf("create %d %s", w.Code, w.Body)
	}
	w = call(app, "GET", "/guide", "", false)
	if w.Code != 302 || w.Header().Get("Location") != "https://example.org/guide" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redirect %d %v", w.Code, w.Header())
	}
	u, err := d.GetURLByShortCode("guide")
	if err != nil || u.Clicks != 1 || u.ExpiresAt == nil {
		t.Fatalf("stored %+v %v", u, err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	expired := model.NewURL("expired", "https://example.org/expired")
	expired.ExpiresAt = &past
	if err := d.SaveURL(expired); err != nil {
		t.Fatal(err)
	}
	w = call(app, "GET", "/expired", "", false)
	if w.Code != 410 || w.Header().Get("Location") != "" {
		t.Fatalf("expired %d %s", w.Code, w.Body)
	}
	u, err = d.GetURLByShortCode("expired")
	if err != nil || u.Clicks != 0 {
		t.Fatalf("expired counted %+v %v", u, err)
	}
	for _, body := range []string{
		`{"url":"https://example.org","expires_at":"2020-01-01T00:00:00Z"}`,
		`{"url":"https://example.org","expires_at":"bad"}`,
		`{"url":"javascript://example.org"}`,
		`{"url":"https://example.org","custom_code":"healthz"}`,
		`{"url":"https://example.org","unknown":1}`,
		`{"url":"https://example.org"} {}`,
	} {
		if w := call(app, "POST", "/api/shorten", body, true); w.Code != 400 {
			t.Errorf("bad body %s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := call(app, "GET", "/delete/guide", "", true); w.Code != 405 {
		t.Fatalf("GET delete: %d", w.Code)
	}
	u, _ = d.GetURLByShortCode("guide")
	if u == nil {
		t.Fatal("GET deleted link")
	}
	r := httptest.NewRequest("POST", "http://example.com/delete/guide", nil)
	r.SetBasicAuth("admin", "test-password")
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	denied := httptest.NewRecorder()
	app.ServeHTTP(denied, r)
	if denied.Code != 403 {
		t.Fatalf("CSRF=%d", denied.Code)
	}
	if w := call(app, "DELETE", "/api/url/guide", "", true); w.Code != 200 {
		t.Fatalf("delete=%d", w.Code)
	}
	if w := call(app, "GET", "/guide", "", false); w.Code != 404 {
		t.Fatalf("deleted redirect=%d", w.Code)
	}
}

func TestConcurrentCustomCodeConflict(t *testing.T) {
	app, _ := realApp(t, false)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- call(app, "POST", "/api/shorten", `{"url":"https://example.com","custom_code":"same"}`, false).Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("conflict responses: %v", counts)
	}
}

func TestRealFormExpiryAndEscaping(t *testing.T) {
	app, d := realApp(t, false)
	req := httptest.NewRequest("POST", "/shorten", strings.NewReader("url=https%3A%2F%2Fexample.com&custom_code=form&ttl_hours=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 303 || w.Header().Get("Location") != "/manage/form" {
		t.Fatalf("form %d %s", w.Code, w.Body)
	}
	for i := 0; i < 2; i++ {
		result := call(app, "GET", "/manage/form", "", false)
		if result.Code != 200 || !strings.Contains(result.Body.String(), "链接已准备好分享") {
			t.Fatalf("result: %d %s", result.Code, result.Body)
		}
	}
	all, err := d.ListURLs()
	if err != nil || len(all) != 1 {
		t.Fatalf("refresh created duplicate rows: %v %v", all, err)
	}
	u, err := d.GetURLByShortCode("form")
	if err != nil || u.ExpiresAt == nil {
		t.Fatalf("expiry %+v %v", u, err)
	}
	w = call(app, "GET", "/urls?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E", "", false)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		t.Fatalf("unsafe HTML: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatalf("missing escaped query: %s", w.Body)
	}
}
