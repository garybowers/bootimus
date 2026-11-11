package server

import (
	"strings"
	"testing"
)

func TestLockedMenuLocalBoot(test *testing.T) {
	for _, scenario := range []struct {
		name    string
		enabled bool
		timeout int
		choice  string
	}{
		{"disabled", false, 10, "choose selected"},
		{"wait", true, 0, "choose --default local --timeout 0 --retimeout 0 selected"},
		{"countdown", true, 10, "choose --default local --timeout 10000 --retimeout 0 selected"},
		{"maximum", true, 86400, "choose --default local --timeout 86400000 --retimeout 0 selected"},
		{"invalid stored value", true, -1, "choose --default local --timeout 0 --retimeout 0 selected"},
		{"oversize stored value", true, 86401, "choose --default local --timeout 0 --retimeout 0 selected"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			server, store, image := newBootAuthServer(test, true)
			theme, err := store.GetMenuTheme()
			if err != nil {
				test.Fatal(err)
			}
			theme.LockedMenuLocalBoot, theme.LockedMenuTimeout = scenario.enabled, scenario.timeout
			theme.DefaultMenuItem, theme.MenuTimeout = "shell", 1
			if err := store.UpdateMenuTheme(theme); err != nil {
				test.Fatal(err)
			}
			if err := store.SetNextBootImage(bootTestMAC, image.Filename); err != nil {
				test.Fatal(err)
			}
			for _, commit := range []bool{false, true} {
				result := server.renderBootMenu(bootTestMAC, "", commit)
				if result.Kind != "locked" || !strings.Contains(result.Script, scenario.choice+" || goto start") {
					test.Fatalf("unexpected locked menu: %s", result.Script)
				}
				if strings.Contains(result.Script, "kernel ") || strings.Contains(result.Script, "item shell") || !strings.Contains(result.Script, "set localboot 1") {
					test.Fatal("locked menu bypasses protection or lacks the local-boot hand-off")
				}
			}
			client, err := store.GetClient(bootTestMAC)
			if err != nil || client.NextBootImage != image.Filename {
				test.Fatal("locked menu consumed next boot")
			}
			server.adminTLS = nil
			if script := server.renderBootMenu(bootTestMAC, "", true).Script; !strings.Contains(script, scenario.choice+" || goto choice") {
				test.Fatal("unavailable HTTPS lost the local-boot setting")
			}
		})
	}
}
