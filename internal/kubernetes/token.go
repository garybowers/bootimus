package kubernetes

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"bootimus/internal/models"
)

const DefaultTokenTTL = 24 * time.Hour

var (
	ErrNotApproved  = errors.New("client not approved for enrolment")
	ErrTokenInvalid = errors.New("invalid enrolment token")
	ErrTokenExpired = errors.New("enrolment token expired")
	ErrUUIDMismatch = errors.New("hardware uuid mismatch")
)

func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ValidateToken(client *models.Client, token string, now time.Time) error {
	if client == nil {
		return ErrNotApproved
	}
	if client.EnrollmentState != models.EnrollmentStateApproved && client.EnrollmentState != models.EnrollmentStateInstalling {
		return ErrNotApproved
	}
	if client.EnrollmentToken == "" || token == "" {
		return ErrTokenInvalid
	}
	if subtle.ConstantTimeCompare([]byte(client.EnrollmentToken), []byte(token)) != 1 {
		return ErrTokenInvalid
	}
	if client.TokenExpiresAt == nil || now.After(*client.TokenExpiresAt) {
		return ErrTokenExpired
	}
	return nil
}

func ValidateUUID(expected, presented string) error {
	if expected == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(presented)) {
		return ErrUUIDMismatch
	}
	return nil
}
