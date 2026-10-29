package admin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func (handler *Handler) updateBootPassword(request *http.Request, password *string, hash *string, required *bool, revision *string) error {
	if password == nil {
		return nil
	}
	var encoded string
	if *password == "" {
		encoded = ""
	} else {
		if request.TLS == nil || handler.BootAuthReady == nil || !handler.BootAuthReady() {
			return fmt.Errorf("boot passwords require admin HTTPS and a certificate trusted by the bootloader")
		}
		if len(*password) > 72 {
			return fmt.Errorf("boot passwords must be at most 72 bytes")
		}
		if strings.ContainsAny(*password, "+; ") {
			return fmt.Errorf("boot passwords must not contain +, ; or spaces (iPXE v2.0.0 compatibility)")
		}
		value, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash boot password: %w", err)
		}
		encoded = string(value)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("generate boot password revision: %w", err)
	}
	*hash = encoded
	*required = encoded != ""
	*revision = hex.EncodeToString(nonce[:])
	return nil
}
