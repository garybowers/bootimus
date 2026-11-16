package models

import (
	"strings"
	"testing"
)

func TestNormaliseBootScript(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		input   string
		want    string
		invalid bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "whitespace only clears", input: " \r\n\t\n", want: ""},
		{name: "windows line endings", input: "\r\nkernel a\r\ninitrd b\r\nboot\r\n", want: "kernel a\ninitrd b\nboot"},
		{name: "old mac line endings", input: "kernel a\rboot", want: "kernel a\nboot"},
		{name: "tabs kept", input: "kernel a\tquiet", want: "kernel a\tquiet"},
		{name: "control character", input: "kernel a\x00", invalid: true},
		{name: "escape character", input: "echo \x1b[2J", invalid: true},
		{name: "invalid utf-8", input: "kernel \xff", invalid: true},
		{name: "at the limit", input: strings.Repeat("a", MaxBootScriptBytes), want: strings.Repeat("a", MaxBootScriptBytes)},
		{name: "over the limit", input: strings.Repeat("a", MaxBootScriptBytes+1), invalid: true},
	} {
		got, err := NormaliseBootScript(scenario.input)
		if scenario.invalid {
			if err == nil {
				t.Errorf("%s: expected an error, got %q", scenario.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", scenario.name, err)
			continue
		}
		if got != scenario.want {
			t.Errorf("%s: got %q, want %q", scenario.name, got, scenario.want)
		}
	}
}
