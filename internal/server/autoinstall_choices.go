package server

import (
	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"crypto/sha256"
	"fmt"
	"strings"
)

func (s *Server) autoInstallChoices(image *models.Image, client *models.Client) []string {
	if image.BootMethod != "kernel" || !models.IsWindowsDistro(image.Distro) {
		return nil
	}
	if client != nil && len(client.AutoInstallChoices) > 0 {
		return client.AutoInstallChoices
	}
	if group := s.clientGroupOf(client); group != nil && len(group.AutoInstallChoices) > 0 {
		return group.AutoInstallChoices
	}
	return image.AutoInstallChoices
}

func autoInstallChoiceID(path string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
}

func (s *Server) resolveAutoInstallChoice(image *models.Image, client *models.Client, choice string) (string, string, string, error) {
	choices := s.autoInstallChoices(image, client)
	if len(choices) == 0 {
		return "", "", "", fmt.Errorf("answer-file selection is not enabled for this image/client")
	}
	if choice == "default" {
		content, kind, source, err := s.resolveAutoInstallScript(image, client)
		if err != nil {
			return "", "", "", err
		}
		if kind != autoinstall.TypeAutounattend {
			return "", "", "", fmt.Errorf("no default Windows answer file")
		}
		return content, kind, source, nil
	}
	for _, path := range choices {
		if autoInstallChoiceID(path) != choice {
			continue
		}
		if s.autoInstallLib == nil || autoinstall.PathType(path) != autoinstall.TypeAutounattend {
			return "", "", "", fmt.Errorf("Windows answer file is unavailable")
		}
		content, err := s.autoInstallLib.ReadPath(path)
		return content, autoinstall.TypeAutounattend, "choice:" + path, err
	}
	return "", "", "", fmt.Errorf("answer file is not offered for this image/client")
}

func (mb *MenuBuilder) buildAutoInstallChoiceMenu(image *models.Image) string {
	choices := mb.autoInstallChoices[image.ID]
	if len(choices) == 0 {
		return ""
	}
	back := "start"
	if image.GroupID != nil && mb.bootAccess.imageID == 0 {
		back = fmt.Sprintf("group%d", *image.GroupID)
	}
	label := func(value string) string {
		return strings.Map(func(r rune) rune {
			if r < 32 || r == 127 || strings.ContainsRune("$&|;", r) {
				return ' '
			}
			return r
		}, value)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "menu --name bootimus_answers Windows answer file: %s || goto failed\n", label(image.Name))
	if mb.autoInstallTypes[image.ID] == autoinstall.TypeAutounattend {
		sb.WriteString("item --menu bootimus_answers default Use configured default || goto failed\n")
	}
	for _, path := range choices {
		fmt.Fprintf(&sb, "item --menu bootimus_answers %s %s || goto failed\n", autoInstallChoiceID(path), label(path))
	}
	sb.WriteString("item --menu bootimus_answers back Back to boot menu || goto failed\n")
	sb.WriteString("clear bootimus_answer\n")
	fmt.Fprintf(&sb, "choose --menu bootimus_answers bootimus_answer || goto %s\n", back)
	fmt.Fprintf(&sb, "iseq ${bootimus_answer} back && goto %s ||\n", back)
	return sb.String()
}
