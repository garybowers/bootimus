package server

import (
	"strings"
	"testing"
)

// Check the interaction at the #242/#243 merge conflict, including the
// password on an image that is visible before global authentication.
func TestLockedMenuTimeoutWithProtectedException(t *testing.T) {
	server, store, image := newBootAuthServer(t, true)
	image.BootMenuExempt = true
	if err := store.UpdateImage(image.Filename, image); err != nil {
		t.Fatal(err)
	}
	theme, err := store.GetMenuTheme()
	if err != nil {
		t.Fatal(err)
	}
	theme.LockedMenuLocalBoot = true
	theme.LockedMenuTimeout = 12
	if err := store.UpdateMenuTheme(theme); err != nil {
		t.Fatal(err)
	}
	script := server.renderBootMenu(bootTestMAC, "", true).Script
	for _, expected := range []string{
		"item public" + strconvImage(image),
		"(image password)",
		"choose --default local --timeout 12000 --retimeout 0 selected",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("locked menu lost %q: %s", expected, script)
		}
	}
	response := lockedImageRequest(server, image).Body.String()
	if !strings.Contains(response, "form Image password") || strings.Contains(response, "kernel ") {
		t.Fatal("exception bypassed its image password")
	}
}
