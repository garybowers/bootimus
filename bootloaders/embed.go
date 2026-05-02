package bootloaders

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
)

//go:embed all:default all:secureboot
var Bootloaders embed.FS

const DefaultSet = "default"

func Resolve(setName, filename string) (data []byte, resolvedSet string, err error) {
	if setName == "" {
		setName = DefaultSet
	}
	p := path.Join(setName, filename)
	if data, err := Bootloaders.ReadFile(p); err == nil {
		return data, setName, nil
	}
	if setName == DefaultSet {
		return nil, "", fmt.Errorf("bootloader file not found: %s", filename)
	}
	p = path.Join(DefaultSet, filename)
	data, err = Bootloaders.ReadFile(p)
	if err != nil {
		return nil, "", fmt.Errorf("bootloader file not found in %q or %q: %s", setName, DefaultSet, filename)
	}
	return data, DefaultSet, nil
}

func ListSets() ([]string, error) {
	entries, err := fs.ReadDir(Bootloaders, ".")
	if err != nil {
		return nil, err
	}
	sets := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			sets = append(sets, e.Name())
		}
	}
	return sets, nil
}

func ListFiles(setName string) ([]fs.DirEntry, error) {
	if setName == "" {
		setName = DefaultSet
	}
	return fs.ReadDir(Bootloaders, setName)
}

func IsBuiltIn(name string) bool {
	if name == "" {
		return false
	}
	sets, err := ListSets()
	if err != nil {
		return false
	}
	for _, s := range sets {
		if s == name {
			return true
		}
	}
	return false
}
