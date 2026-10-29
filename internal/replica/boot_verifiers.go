package replica

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"bootimus/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type bootVerifier struct {
	Filename  string    `json:"filename"`
	UpdatedAt time.Time `json:"updated_at"`
	Required  bool      `json:"required"`
	Hash      string    `json:"hash"`
	Revision  string    `json:"revision"`
}

type bootVerifierFeed struct {
	Version int            `json:"version"`
	Images  []bootVerifier `json:"images"`
	Theme   bootVerifier   `json:"theme"`
}

func (s *Syncer) bootVerifiers(ctx context.Context) (*bootVerifierFeed, error) {
	var feed bootVerifierFeed
	if err := s.primary.getBootVerifiers(ctx, &feed); err != nil {
		return nil, err
	}
	if feed.Version != 1 {
		return nil, fmt.Errorf("unsupported boot verifier feed version %d", feed.Version)
	}
	return &feed, nil
}

func validBootHash(hash string) bool {
	if len(hash) != 60 {
		return false
	}
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost <= 14
}

func validVerifier(verifier bootVerifier, updatedAt time.Time, required bool, revision string) bool {
	if verifier.UpdatedAt.IsZero() || !verifier.UpdatedAt.Equal(updatedAt) || verifier.Required != required || verifier.Revision != revision {
		return false
	}
	if revision != "" {
		if len(revision) != 32 {
			return false
		}
		if _, err := hex.DecodeString(revision); err != nil {
			return false
		}
	}
	if !required {
		return verifier.Hash == ""
	}
	return validBootHash(verifier.Hash)
}

func sameProtectedPolicy(required bool, hash, localRevision string, sourceAt time.Time, remoteRevision string, remoteUpdatedAt time.Time) bool {
	if !required || !validBootHash(hash) {
		return false
	}
	if remoteRevision != "" {
		return localRevision == remoteRevision
	}
	return localRevision == "" && !sourceAt.IsZero() && sourceAt.Equal(remoteUpdatedAt)
}

func (feed *bootVerifierFeed) imageHashes(images []models.Image) (map[string]string, error) {
	if len(feed.Images) != len(images) {
		return nil, fmt.Errorf("boot verifier feed does not match image metadata")
	}
	byFilename := make(map[string]bootVerifier, len(feed.Images))
	for _, verifier := range feed.Images {
		if verifier.Filename == "" {
			return nil, fmt.Errorf("boot verifier feed has an unnamed image")
		}
		if _, duplicate := byFilename[verifier.Filename]; duplicate {
			return nil, fmt.Errorf("boot verifier feed has duplicate images")
		}
		byFilename[verifier.Filename] = verifier
	}
	hashes := make(map[string]string, len(images))
	for _, image := range images {
		verifier, ok := byFilename[image.Filename]
		if !ok || !validVerifier(verifier, image.UpdatedAt, image.BootPasswordRequired, image.BootPasswordRevision) {
			return nil, fmt.Errorf("boot verifier feed does not match image %q", image.Filename)
		}
		hashes[image.Filename] = verifier.Hash
	}
	return hashes, nil
}

func (s *Syncer) guardImageBootPolicies(remote []models.Image, local []*models.Image) error {
	byFilename := make(map[string]*models.Image, len(local))
	for _, image := range local {
		byFilename[image.Filename] = image
	}
	for _, image := range remote {
		if !image.BootPasswordRequired {
			continue
		}
		current := byFilename[image.Filename]
		if current == nil || sameProtectedPolicy(current.BootPasswordRequired, current.BootPasswordHash, current.BootPasswordRevision, current.BootPasswordSourceAt, image.BootPasswordRevision, image.UpdatedAt) {
			continue
		}
		current.BootPasswordRequired = true
		current.BootPasswordHash = ""
		current.BootPasswordRevision = ""
		current.BootPasswordSourceAt = time.Time{}
		if err := s.deps.Store.UpdateImage(current.Filename, current); err != nil {
			return fmt.Errorf("block image %q pending boot verifier: %w", current.Filename, err)
		}
	}
	return nil
}

func (s *Syncer) guardThemeBootPolicy(remote models.MenuTheme, local *models.MenuTheme) error {
	if !remote.BootPasswordRequired || sameProtectedPolicy(local.BootPasswordRequired, local.BootPasswordHash, local.BootPasswordRevision, local.BootPasswordSourceAt, remote.BootPasswordRevision, remote.UpdatedAt) {
		return nil
	}
	local.BootPasswordRequired = true
	local.BootPasswordHash = ""
	local.BootPasswordRevision = ""
	local.BootPasswordSourceAt = time.Time{}
	if err := s.deps.Store.UpdateMenuTheme(local); err != nil {
		return fmt.Errorf("block menu pending boot verifier: %w", err)
	}
	return nil
}
