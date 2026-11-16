package models

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBootScriptBytes = 16 * 1024

func NormaliseBootScript(script string) (string, error) {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	script = strings.ReplaceAll(script, "\r", "\n")
	script = strings.TrimSpace(script)
	if len(script) > MaxBootScriptBytes {
		return "", fmt.Errorf("boot script must be at most %d bytes", MaxBootScriptBytes)
	}
	if !utf8.ValidString(script) {
		return "", fmt.Errorf("boot script must be valid UTF-8")
	}
	for _, character := range script {
		if character != '\n' && character != '\t' && unicode.IsControl(character) {
			return "", fmt.Errorf("boot script must not contain control characters")
		}
	}
	return script, nil
}
