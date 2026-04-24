package autoinstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrInvalidName = errors.New("invalid name")
	ErrNotFound    = errors.New("autoinstall file not found")
)

type Library struct {
	root string
}

func New(dataDir string) (*Library, error) {
	root := filepath.Join(dataDir, "autoinstall")
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create autoinstall dir: %w", err)
	}
	return &Library{root: root}, nil
}

func (l *Library) Root() string { return l.root }

type File struct {
	Distro   string `json:"distro"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
}

func scriptTypeFromExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xml":
		return "autounattend"
	case ".cfg":
		return "preseed"
	case ".ks":
		return "kickstart"
	case ".yaml", ".yml":
		return "autoinstall"
	default:
		return "generic"
	}
}

func (l *Library) List() ([]File, error) {
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return nil, err
	}
	var out []File
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		distro := e.Name()
		inner, err := os.ReadDir(filepath.Join(l.root, distro))
		if err != nil {
			continue
		}
		for _, f := range inner {
			if f.IsDir() {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			out = append(out, File{
				Distro:   distro,
				Filename: f.Name(),
				Path:     filepath.ToSlash(filepath.Join(distro, f.Name())),
				Type:     scriptTypeFromExt(f.Name()),
				Size:     info.Size(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Distro != out[j].Distro {
			return out[i].Distro < out[j].Distro
		}
		return out[i].Filename < out[j].Filename
	})
	return out, nil
}

func (l *Library) Read(distro, filename string) (string, error) {
	if err := validateName(distro); err != nil {
		return "", err
	}
	if err := validateName(filename); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(l.root, distro, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return string(b), nil
}

func (l *Library) ReadPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", ErrNotFound
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) != 2 {
		return "", ErrInvalidName
	}
	return l.Read(parts[0], parts[1])
}

func (l *Library) Write(distro, filename, content string) error {
	if err := validateName(distro); err != nil {
		return err
	}
	if err := validateName(filename); err != nil {
		return err
	}
	dir := filepath.Join(l.root, distro)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create distro dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

func (l *Library) Delete(distro, filename string) error {
	if err := validateName(distro); err != nil {
		return err
	}
	if err := validateName(filename); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(l.root, distro, filename)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty", ErrInvalidName)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%w: separator", ErrInvalidName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: traversal", ErrInvalidName)
	}
	return nil
}
