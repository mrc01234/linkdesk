package database

import (
	"context"
	"database/sql"
	"errors"
	"linkdesk/model"
	"path/filepath"
	"testing"
	"time"
)

func TestPaginationSearchAndExpiry(t *testing.T) {
	d, cleanup := setupTestDB(t)
	defer cleanup()
	now := time.Now().UTC().Truncate(time.Second)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	for i, spec := range []struct {
		code, target string
		expiry       *time.Time
	}{
		{"alpha", "https://example.com/docs", nil},
		{"beta", "https://example.com/go", &future},
		{"gamma", "https://example.org/old", &past},
		{"percent", "https://example.com/100%_done", nil},
	} {
		u := &model.URL{ShortCode: spec.code, LongURL: spec.target, CreatedAt: now, ExpiresAt: spec.expiry}
		if err := d.SaveURL(u); err != nil {
			t.Fatal(i, err)
		}
	}
	page, err := d.SearchURLs(context.Background(), model.ListOptions{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || page.TotalPages != 2 || len(page.URLs) != 2 || page.URLs[0].ShortCode != "percent" || page.URLs[1].ShortCode != "gamma" {
		t.Fatalf("page1: %+v", page)
	}
	page, err = d.SearchURLs(context.Background(), model.ListOptions{Page: 2, PageSize: 2})
	if err != nil || len(page.URLs) != 2 || page.URLs[0].ShortCode != "beta" || page.URLs[1].ShortCode != "alpha" {
		t.Fatalf("page2: %+v, %v", page, err)
	}
	for _, tc := range []struct {
		q, status string
		total     int
	}{
		{"EXAMPLE.COM", "all", 3}, {"ALPHA", "", 1}, {"%_", "", 1},
		{"' OR 1=1 --", "", 0}, {"", "active", 3}, {"", "expired", 1},
	} {
		p, err := d.SearchURLs(context.Background(), model.ListOptions{Query: tc.q, Status: tc.status, Page: 1, PageSize: 10})
		if err != nil || p.Total != tc.total || p.URLs == nil {
			t.Fatalf("%+v => %+v, %v", tc, p, err)
		}
	}
	page, err = d.SearchURLs(context.Background(), model.ListOptions{Page: 99, PageSize: 2})
	if err != nil || page.Total != 4 || page.URLs == nil || len(page.URLs) != 0 {
		t.Fatalf("out of range: %+v %v", page, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.SearchURLs(ctx, model.ListOptions{Page: 1, PageSize: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query: %v", err)
	}
	if err := d.SaveURL(model.NewURL("alpha", "https://another.example")); err == nil {
		t.Fatal("duplicate accepted")
	} else {
		var duplicate *model.ErrCustomCodeAlreadyExists
		if !errors.As(err, &duplicate) {
			t.Fatalf("wrong duplicate error: %v", err)
		}
	}
}

func TestMigrationPreservesUpstreamDataAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE urls(id INTEGER PRIMARY KEY AUTOINCREMENT, short_code TEXT UNIQUE NOT NULL, long_url TEXT NOT NULL, created_at TIMESTAMP NOT NULL, clicks INTEGER NOT NULL DEFAULT 0);
 INSERT INTO urls(short_code,long_url,created_at,clicks) VALUES('legacy','https://example.com','2026-01-01 00:00:00',7)`)
	old.Close()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := d.GetURLByShortCode("legacy")
	if err != nil || u == nil || u.Clicks != 7 || u.ExpiresAt != nil {
		t.Fatalf("legacy: %+v %v", u, err)
	}
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	fresh := model.NewURL("new", "https://example.org")
	fresh.ExpiresAt = &expiry
	if err := d.SaveURL(fresh); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u, err = d.GetURLByShortCode("new")
	if err != nil || u == nil || u.ExpiresAt == nil || !u.ExpiresAt.Equal(expiry) {
		t.Fatalf("reopened: %+v %v", u, err)
	}
	if _, err := d.SearchURLs(context.Background(), model.ListOptions{Page: 0, PageSize: 10}); err == nil {
		t.Fatal("invalid page accepted")
	}
}
