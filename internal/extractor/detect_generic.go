package extractor

import (
	"fmt"
	"log"
	"strings"
)

func (e *Extractor) detectGenericUnified(reader FileSystemReader) (*BootFiles, error) {
	log.Printf("Generic boot file scanner: scanning ISO filesystem...")

	var kernelCandidates []string
	var initrdCandidates []string

	walkDir(reader, "/", 0, 5, func(path string, isDir bool) {
		if isDir {
			return
		}
		lower := strings.ToLower(path)
		name := lower
		if idx := strings.LastIndex(lower, "/"); idx >= 0 {
			name = lower[idx+1:]
		}

		if isKernelFile(name) {
			kernelCandidates = append(kernelCandidates, path)
		}

		if isInitrdFile(name) {
			initrdCandidates = append(initrdCandidates, path)
		}
	})

	log.Printf("Generic scanner found %d kernel candidates: %v", len(kernelCandidates), kernelCandidates)
	log.Printf("Generic scanner found %d initrd candidates: %v", len(initrdCandidates), initrdCandidates)

	if len(kernelCandidates) == 0 {
		return nil, fmt.Errorf("no kernel files found by generic scanner")
	}
	if len(initrdCandidates) == 0 {
		return nil, fmt.Errorf("kernel found but no initrd files found by generic scanner")
	}

	kernel, initrd := pickBestPair(kernelCandidates, initrdCandidates)

	log.Printf("Generic scanner selected: kernel=%s initrd=%s", kernel, initrd)

	bootParams := guessBootParams(reader, kernel)

	return &BootFiles{
		Kernel:     kernel,
		Initrd:     initrd,
		Distro:     "generic",
		BootParams: bootParams,
	}, nil
}

func isKernelFile(name string) bool {
	kernelPatterns := []string{
		"vmlinuz",
		"bzimage",
		"linux",
	}
	for _, p := range kernelPatterns {
		if name == p || strings.HasPrefix(name, p+"-") || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
}

func isInitrdFile(name string) bool {
	initrdPatterns := []string{
		"initrd",
		"initramfs",
	}
	for _, p := range initrdPatterns {
		if name == p || strings.HasPrefix(name, p+"-") || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
}

func walkDir(reader FileSystemReader, path string, depth, maxDepth int, fn func(path string, isDir bool)) {
	if depth > maxDepth {
		return
	}

	entries, err := reader.ListDirectory(path)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.Name == "" || entry.Name == "." || entry.Name == ".." {
			continue
		}

		fullPath := path
		if fullPath == "/" {
			fullPath = "/" + entry.Name
		} else {
			fullPath = path + "/" + entry.Name
		}

		fn(fullPath, entry.IsDir)

		if entry.IsDir {
			walkDir(reader, fullPath, depth+1, maxDepth, fn)
		}
	}
}

func pickBestPair(kernels, initrds []string) (string, string) {
	for _, k := range kernels {
		kDir := parentDir(k)
		for _, i := range initrds {
			if parentDir(i) == kDir {
				return k, i
			}
		}
	}
	return kernels[0], initrds[0]
}

func parentDir(path string) string {
	if idx := strings.LastIndex(path, "/"); idx > 0 {
		return path[:idx]
	}
	return "/"
}

func guessBootParams(reader FileSystemReader, kernelPath string) string {
	_ = kernelPath

	syslinuxPaths := []string{
		"/boot/syslinux/syslinux.cfg",
		"/syslinux/syslinux.cfg",
		"/isolinux/isolinux.cfg",
		"/boot/grub/grub.cfg",
	}
	for _, cfgPath := range syslinuxPaths {
		content := reader.ReadFileContent(cfgPath)
		if content != "" {
			if params := extractBootParamsFromConfig(content, kernelPath); params != "" {
				return params
			}
		}
	}

	return ""
}

func extractBootParamsFromConfig(config, kernelPath string) string {
	kernelName := kernelPath
	if idx := strings.LastIndex(kernelPath, "/"); idx >= 0 {
		kernelName = kernelPath[idx+1:]
	}

	lines := strings.Split(config, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))

		if (strings.HasPrefix(trimmed, "kernel ") || strings.HasPrefix(trimmed, "linux ")) &&
			strings.Contains(trimmed, strings.ToLower(kernelName)) {
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				appendLine := strings.TrimSpace(lines[j])
				if strings.HasPrefix(strings.ToLower(appendLine), "append ") {
					params := appendLine[7:]
					params = removeInitrdParam(params)
					return strings.TrimSpace(params) + " "
				}
			}
		}

		if strings.HasPrefix(trimmed, "linux ") && strings.Contains(trimmed, strings.ToLower(kernelName)) {
			parts := strings.Fields(line)
			if len(parts) > 2 {
				params := strings.Join(parts[2:], " ")
				params = removeInitrdParam(params)
				return strings.TrimSpace(params) + " "
			}
		}
	}

	return ""
}

func removeInitrdParam(params string) string {
	var result []string
	for _, p := range strings.Fields(params) {
		if !strings.HasPrefix(strings.ToLower(p), "initrd=") {
			result = append(result, p)
		}
	}
	return strings.Join(result, " ")
}
