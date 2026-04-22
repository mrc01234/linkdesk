package service

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/mstgnz/self-hosted-url-shortener/database"
	"github.com/mstgnz/self-hosted-url-shortener/model"
	"github.com/mstgnz/self-hosted-url-shortener/pkg/util"
	"github.com/skip2/go-qrcode"
)

const shortCodeLength = 6
const maxCustomCodeLength = 50

var reservedCodes = map[string]struct{}{
	"api": {}, "static": {}, "urls": {}, "shorten": {},
	"qr": {}, "delete": {}, "favicon.ico": {},
}

// URLService handles the business logic for URL shortening
type URLService struct {
	db database.DatabaseInterface
}

// New creates a new URL service
func New(db database.DatabaseInterface) *URLService {
	return &URLService{db: db}
}

// ShortenURL creates a shortened URL
func (s *URLService) ShortenURL(longURL, customCode string) (*model.URL, error) {
	if !strings.HasPrefix(longURL, "http://") && !strings.HasPrefix(longURL, "https://") {
		longURL = "https://" + longURL
	}

	if !util.IsValidURL(longURL) {
		return nil, &model.ErrInvalidURL{URL: longURL}
	}

	var shortCode string
	if customCode != "" {
		if err := validateCustomCode(customCode); err != nil {
			return nil, err
		}

		existingURL, err := s.db.GetURLByShortCode(customCode)
		if err != nil {
			return nil, fmt.Errorf("error checking custom code: %w", err)
		}
		if existingURL != nil {
			return nil, &model.ErrCustomCodeAlreadyExists{Code: customCode}
		}
		shortCode = customCode
	} else {
		var err error
		shortCode, err = generateShortCode(shortCodeLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate short code: %w", err)
		}

		for {
			existingURL, err := s.db.GetURLByShortCode(shortCode)
			if err != nil {
				return nil, fmt.Errorf("error checking short code: %w", err)
			}
			if existingURL == nil {
				break
			}
			shortCode, err = generateShortCode(shortCodeLength)
			if err != nil {
				return nil, fmt.Errorf("failed to generate short code: %w", err)
			}
		}
	}

	url := model.NewURL(shortCode, longURL)
	if err := s.db.SaveURL(url); err != nil {
		return nil, fmt.Errorf("failed to save URL: %w", err)
	}

	return url, nil
}

// GetURL retrieves a URL by its short code
func (s *URLService) GetURL(shortCode string) (*model.URL, error) {
	url, err := s.db.GetURLByShortCode(shortCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}
	return url, nil
}

// RecordClick records a click on a URL
func (s *URLService) RecordClick(shortCode string) error {
	return s.db.IncrementClicks(shortCode)
}

// ListURLs retrieves all URLs
func (s *URLService) ListURLs() ([]*model.URL, error) {
	return s.db.ListURLs()
}

// DeleteURL deletes a URL by its short code
func (s *URLService) DeleteURL(shortCode string) error {
	return s.db.DeleteURL(shortCode)
}

// GenerateQRCode generates a QR code for a URL
func (s *URLService) GenerateQRCode(shortURL string) ([]byte, error) {
	qr, err := qrcode.Encode(shortURL, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}
	return qr, nil
}

// validateCustomCode checks that a custom code is safe to use
func validateCustomCode(code string) error {
	if len(code) > maxCustomCodeLength {
		return &model.ErrInvalidCode{
			Code:   code,
			Reason: fmt.Sprintf("exceeds maximum length of %d characters", maxCustomCodeLength),
		}
	}
	for _, r := range code {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return &model.ErrInvalidCode{
				Code:   code,
				Reason: fmt.Sprintf("contains invalid character '%c': only letters, digits, hyphens and underscores are allowed", r),
			}
		}
	}
	if _, reserved := reservedCodes[strings.ToLower(code)]; reserved {
		return &model.ErrInvalidCode{
			Code:   code,
			Reason: "this path is reserved",
		}
	}
	return nil
}

// generateShortCode generates a random short code of the specified length
func generateShortCode(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	for i, b := range randomBytes {
		randomBytes[i] = charset[b%byte(len(charset))]
	}

	return string(randomBytes), nil
}
