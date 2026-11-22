package admin

import (
	"strings"
	"testing"
)

func TestBuildStartnetScriptUnattendLookup(t *testing.T) {
	cases := []struct {
		name        string
		autoInstall bool
		wantShare   bool
	}{
		{"no image-level file", false, false},
		{"image-level file staged", true, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script := buildStartnetScript("10.0.0.1", "win11", 445, 8080, "Win11.iso", c.autoInstall)
			if script == "" {
				t.Fatal("expected a rendered startnet.cmd")
			}
			injected := `if exist X:\Windows\System32\AutoUnattend.xml (`
			register := `reg add HKLM\SYSTEM\Setup /v UnattendFile /t REG_SZ /d "!UNATTEND!" /f`
			fallback := `if not defined UNATTEND if exist Z:\AutoUnattend.xml (`
			apply := `if defined UNATTEND set SETUP_ARGS=!SETUP_ARGS! /unattend:!UNATTEND!`

			_, block, ok := strings.Cut(script, injected)
			if !ok {
				t.Fatal("missing check for the wimboot-injected file")
			}
			block, _, ok = strings.Cut(block, "\n)")
			if !ok {
				t.Fatal("missing end of the injected-file check")
			}
			assign := strings.Index(block, `set "UNATTEND=X:\Windows\System32\AutoUnattend.xml"`)
			if assign < 0 || strings.Index(block, register) <= assign || strings.Count(script, "/v UnattendFile ") != 1 {
				t.Fatalf("register only the injected answer file, after selecting its path:\n%s", block)
			}
			if got := strings.Contains(script, fallback); got != c.wantShare {
				t.Errorf("SMB fallback present=%v, want %v:\n%s", got, c.wantShare, script)
			}
			if !strings.Contains(script, apply) {
				t.Errorf("expected /unattend to be applied from UNATTEND:\n%s", script)
			}
			if strings.Index(script, injected) > strings.Index(script, apply) {
				t.Errorf("injected check must run before /unattend is applied:\n%s", script)
			}
			if c.wantShare && strings.Index(script, injected) > strings.Index(script, fallback) {
				t.Errorf("wimboot-injected file must take precedence over the share copy:\n%s", script)
			}
			if !strings.Contains(script, `\\10.0.0.1\win11`) {
				t.Errorf("expected the share path in the script:\n%s", script)
			}
		})
	}
}
