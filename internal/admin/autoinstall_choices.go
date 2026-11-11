package admin

import (
	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"fmt"
	"strings"
)

func (h *Handler) autoInstallChoicesFromPayload(raw interface{}) (models.StringSlice, error) {
	values, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("auto_install_choices must be an array of file paths")
	}
	choices := make(models.StringSlice, 0, len(values))
	for _, value := range values {
		path, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("auto_install_choices must contain only file paths")
		}
		choices = append(choices, path)
	}
	return choices, h.validateAutoInstallChoices(choices)
}

func (h *Handler) validateAutoInstallChoices(choices []string) error {
	if len(choices) > 32 {
		return fmt.Errorf("select at most 32 Windows answer files")
	}
	seen := make(map[string]bool)
	for _, path := range choices {
		if path == "" || len(path) > 512 || strings.TrimSpace(path) != path || autoinstall.PathType(path) != autoinstall.TypeAutounattend {
			return fmt.Errorf("invalid Windows answer file: %q", path)
		}
		if seen[path] {
			return fmt.Errorf("duplicate answer file: %q", path)
		}
		seen[path] = true
		if h.autoInstallLib == nil {
			return fmt.Errorf("auto-install file library is unavailable")
		}
		if _, err := h.autoInstallLib.ReadPath(path); err != nil {
			return fmt.Errorf("read answer file %q: %w", path, err)
		}
	}
	return nil
}
