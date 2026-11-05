package service

import (
	"context"
	"linkdesk/model"
	"time"
)

// URLServiceInterface defines the interface for URL service operations
type URLServiceInterface interface {
	// ShortenURL creates a shortened URL
	ShortenURL(longURL, customCode string, expiresAt ...*time.Time) (*model.URL, error)

	// GetURL retrieves a URL by its short code
	GetURL(shortCode string) (*model.URL, error)

	// RecordClick records a click for a URL
	RecordClick(shortCode string) error

	// ListURLs returns all URLs
	ListURLs() ([]*model.URL, error)
	SearchURLs(context.Context, model.ListOptions) (*model.URLPage, error)

	// DeleteURL deletes a URL
	DeleteURL(shortCode string) error

	// GenerateQRCode generates a QR code for a URL
	GenerateQRCode(shortURL string) ([]byte, error)
}
