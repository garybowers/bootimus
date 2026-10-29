package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"

	"golang.org/x/crypto/bcrypt"
)

func TestBootPasswordUpdatesAndRedaction(test *testing.T) {
	for _, target := range []string{"menu", "image"} {
		test.Run(target, func(test *testing.T) {
			store := storagetest.New(test)
			image := &models.Image{Name: "Test", Filename: "test.iso", Enabled: true, Public: true}
			if err := store.CreateImage(image); err != nil {
				test.Fatal(err)
			}
			handler := &Handler{storage: store, BootAuthReady: func() bool { return true }}
			update := func(body, scheme string) *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPut, scheme+"://bootimus/api/images?filename=test.iso", strings.NewReader(body))
				recorder := httptest.NewRecorder()
				if target == "image" {
					handler.UpdateImage(recorder, request)
				} else {
					handler.UpdateMenuTheme(recorder, request)
				}
				return recorder
			}
			read := func() (string, bool) {
				if target == "image" {
					current, err := store.GetImage("test.iso")
					if err != nil {
						test.Fatal(err)
					}
					return current.BootPasswordHash, current.BootPasswordRequired
				}
				current, err := store.GetMenuTheme()
				if err != nil {
					test.Fatal(err)
				}
				return current.BootPasswordHash, current.BootPasswordRequired
			}
			readRevision := func() string {
				if target == "image" {
					current, err := store.GetImage("test.iso")
					if err != nil {
						test.Fatal(err)
					}
					return current.BootPasswordRevision
				}
				current, err := store.GetMenuTheme()
				if err != nil {
					test.Fatal(err)
				}
				return current.BootPasswordRevision
			}
			response := update(`{"boot_password":"secret&=%"}`, "http")
			if response.Code != http.StatusBadRequest {
				test.Fatal("password accepted without HTTPS")
			}
			response = update(`{"boot_password":"secret&=%"}`, "https")
			if response.Code != http.StatusOK {
				test.Fatalf("set password: %d", response.Code)
			}
			hash, required := read()
			revision := readRevision()
			if !required || revision == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret&=%")) != nil {
				test.Fatal("password was not hashed and stored")
			}
			for _, forbidden := range []string{"secret&=%", hash, "boot_password_hash"} {
				if strings.Contains(response.Body.String(), forbidden) {
					test.Fatal("API response exposes password material")
				}
			}
			response = update(`{"name":"Renamed","title":"Renamed","boot_password_required":false,"boot_password_hash":"injected"}`, "https")
			retained, required := read()
			if response.Code != http.StatusOK || retained != hash || !required || readRevision() != revision {
				test.Fatal("ordinary edit or derived fields removed protection")
			}
			body, err := json.Marshal(map[string]string{"boot_password": strings.Repeat("x", 73)})
			if err != nil {
				test.Fatal(err)
			}
			if response = update(string(body), "https"); response.Code != http.StatusBadRequest {
				test.Fatal("overlong bcrypt password accepted")
			}
			if retained, _ := read(); retained != hash {
				test.Fatal("failed edit changed stored password")
			}
			for _, password := range []string{"secret+word", "secret;word", "secret word", " leading", "trailing ", "+", " "} {
				body, err := json.Marshal(map[string]string{"boot_password": password})
				if err != nil {
					test.Fatal(err)
				}
				response := update(string(body), "https")
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "must not contain +, ; or spaces") {
					test.Fatalf("incompatible password accepted: %d %s", response.Code, response.Body.String())
				}
				if retained, required := read(); retained != hash || !required || readRevision() != revision {
					test.Fatal("rejected password changed protection")
				}
			}
			if response = update(`{"boot_password":"replacement"}`, "https"); response.Code != http.StatusOK {
				test.Fatal("password rotation failed")
			}
			if retained, _ := read(); retained == hash || readRevision() == revision {
				test.Fatal("rotation kept the old hash")
			}
			revision = readRevision()
			if response = update(`{"boot_password":""}`, "https"); response.Code != http.StatusOK {
				test.Fatal("password removal failed")
			}
			if retained, required := read(); retained != "" || required || readRevision() == revision {
				test.Fatal("password removal did not persist")
			}
		})
	}
}

func TestBootPasswordRequiresActiveTLSListener(test *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodPut, "https://bootimus/api/theme", nil)
	password, hash, revision, required := "secret", "", "", false
	if err := handler.updateBootPassword(request, &password, &hash, &required, &revision); err == nil {
		test.Fatal("accepted a password without an active authentication listener")
	}
}
