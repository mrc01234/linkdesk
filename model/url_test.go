package model

import (
	"testing"
	"time"
)

func TestExpiryBoundary(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u := NewURL("x", "https://example.com")
	if u.Expired(now) {
		t.Fatal("permanent expired")
	}
	u.ExpiresAt = &now
	if !u.Expired(now) || u.Expired(now.Add(-time.Second)) {
		t.Fatal("expiry boundary")
	}
}
