package model

import (
	"time"
)

// URL represents a shortened URL in the database
type URL struct {
	ID        int64      `json:"id"`
	ShortCode string     `json:"short_code"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	Clicks    int64      `json:"clicks"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Expired treats the exact expiry second as expired. A nil expiry never expires.
func (u *URL) Expired(now time.Time) bool {
	return u.ExpiresAt != nil && !now.Before(*u.ExpiresAt)
}

type ListOptions struct {
	Query    string
	Status   string
	Page     int
	PageSize int
}

type URLPage struct {
	URLs       []*URL `json:"urls"`
	Total      int    `json:"total"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	TotalPages int    `json:"total_pages"`
}

type ErrInvalidInput struct{ Message string }

func (e *ErrInvalidInput) Error() string { return e.Message }

// NewURL creates a new URL with default values
func NewURL(shortCode, longURL string) *URL {
	return &URL{
		ShortCode: shortCode,
		LongURL:   longURL,
		CreatedAt: time.Now(),
		Clicks:    0,
	}
}
