package wim

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Manager struct {
	wimlibPath string
}

func NewManager() (*Manager, error) {
	wimlibPath, err := exec.LookPath("wimlib-imagex")
	if err != nil {
		return nil, fmt.Errorf("wimlib-imagex not found in PATH: %w", err)
	}

	return &Manager{
		wimlibPath: wimlibPath,
	}, nil
}

func IsAvailable() bool {
	_, err := exec.LookPath("wimlib-imagex")
	return err == nil
}

func (m *Manager) InjectDrivers(wimPath string, driverPaths []string, imageIndex int) error {
	if len(driverPaths) == 0 {
		return nil
	}

	log.Printf("Injecting %d driver pack(s) into %s (index %d)", len(driverPaths), wimPath, imageIndex)

	mountDir, err := os.MkdirTemp("", "wim-mount-*")
	if err != nil {
		return fmt.Errorf("failed to create temp mount directory: %w", err)
	}
	defer os.RemoveAll(mountDir)

	log.Printf("Mounting WIM image...")
	mountCmd := exec.Command(m.wimlibPath, "mount", wimPath, fmt.Sprintf("%d", imageIndex), mountDir)
	if output, err := mountCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to mount WIM: %w\nOutput: %s", err, string(output))
	}

	defer func() {
		log.Printf("Unmounting WIM image...")
		unmountCmd := exec.Command(m.wimlibPath, "unmount", mountDir, "--commit")
		if output, err := unmountCmd.CombinedOutput(); err != nil {
			log.Printf("Failed to unmount WIM: %v\nOutput: %s", err, string(output))
		}
	}()

	driversDir := filepath.Join(mountDir, "Windows", "System32", "drivers")
	if err := os.MkdirAll(driversDir, 0755); err != nil {
		return fmt.Errorf("failed to create drivers directory in WIM: %w", err)
	}

	for _, driverPath := range driverPaths {
		log.Printf("Injecting driver pack: %s", filepath.Base(driverPath))

		ext := strings.ToLower(filepath.Ext(driverPath))
		if ext == ".zip" || ext == ".7z" || ext == ".tar" || ext == ".gz" {
			tempExtractDir, err := os.MkdirTemp("", "driver-extract-*")
			if err != nil {
				return fmt.Errorf("failed to create temp extract directory: %w", err)
			}
			defer os.RemoveAll(tempExtractDir)

			extractCmd := exec.Command("7z", "x", driverPath, fmt.Sprintf("-o%s", tempExtractDir), "-y")
			if output, err := extractCmd.CombinedOutput(); err != nil {
				log.Printf("Warning: Failed to extract driver pack %s: %v\nOutput: %s", driverPath, err, string(output))
				continue
			}

			if err := copyDir(tempExtractDir, driversDir); err != nil {
				log.Printf("Warning: Failed to copy extracted drivers: %v", err)
				continue
			}
		} else {
			destPath := filepath.Join(driversDir, filepath.Base(driverPath))
			if err := copyFile(driverPath, destPath); err != nil {
				log.Printf("Warning: Failed to copy driver file %s: %v", driverPath, err)
				continue
			}
		}
	}

	log.Printf("Driver injection complete")
	return nil
}

func (m *Manager) GetImageCount(wimPath string) (int, error) {
	cmd := exec.Command(m.wimlibPath, "info", wimPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get WIM info: %w\nOutput: %s", err, string(output))
	}

	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Index:") {
			count++
		}
	}

	return count, nil
}

const wimlibPathMissing = 49

func (m *Manager) CarriesBootFiles(wimPath string, index int) (bool, error) {
	if index <= 0 {
		bootIndex, err := m.bootIndex(wimPath)
		if err != nil {
			return false, err
		}
		index = bootIndex
	}
	bcd, err := m.hasPath(wimPath, index, "/Windows/Boot/DVD/PCAT/BCD")
	if err != nil || !bcd {
		return false, err
	}
	for _, sdi := range []string{"/Windows/Boot/DVD/PCAT/boot.sdi", "/sms/boot/boot.sdi"} {
		found, err := m.hasPath(wimPath, index, sdi)
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
}

func (m *Manager) bootIndex(wimPath string) (int, error) {
	output, err := exec.Command(m.wimlibPath, "info", wimPath).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("read WIM info: %w: %s", err, strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		value, found := strings.CutPrefix(strings.TrimSpace(line), "Boot Index:")
		if !found {
			continue
		}
		if index, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && index > 0 {
			return index, nil
		}
	}
	return 1, nil
}

func (m *Manager) hasPath(wimPath string, index int, path string) (bool, error) {
	cmd := exec.Command(m.wimlibPath, "dir", wimPath, strconv.Itoa(index), "--path="+path)
	cmd.Env = append(os.Environ(), "WIMLIB_IMAGEX_IGNORE_CASE=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == wimlibPathMissing {
		return false, nil
	}
	return false, fmt.Errorf("list %s in WIM: %w: %s", path, err, strings.TrimSpace(string(output)))
}

func (m *Manager) PatchStartnetCmd(wimPath, content string) error {
	startnetTmp, err := writeTempCRLF("startnet-*.cmd", content)
	if err != nil {
		return fmt.Errorf("failed to stage startnet.cmd: %w", err)
	}
	defer os.Remove(startnetTmp)

	winpeshlTmp, err := writeTempCRLF("winpeshl-*.ini",
		"[LaunchApps]\n%SYSTEMROOT%\\System32\\cmd.exe, /c %SYSTEMROOT%\\System32\\startnet.cmd\n")
	if err != nil {
		return fmt.Errorf("failed to stage winpeshl.ini: %w", err)
	}
	defer os.Remove(winpeshlTmp)

	script := fmt.Sprintf(
		"add %s /Windows/System32/startnet.cmd\nadd %s /Windows/System32/winpeshl.ini\n",
		startnetTmp, winpeshlTmp,
	)

	imageCount, err := m.GetImageCount(wimPath)
	if err != nil {
		return fmt.Errorf("failed to get WIM image count: %w", err)
	}

	// Validate that we have one or two images.
	// NB In Windows PE ISO boot.wim: only one image (index 1) is present.
	// NB In Windows Setup ISO boot.wim: two images (index 1 and 2) are present.
	if imageCount < 1 || imageCount > 2 {
		return fmt.Errorf("unexpected WIM image count: %d (expected 1 or 2)", imageCount)
	}

	cmd := exec.Command(m.wimlibPath, "update", wimPath, strconv.Itoa(imageCount), "--rebuild")
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wimlib-imagex update failed: %w\nOutput: %s", err, string(output))
	}

	log.Printf("WIM: patched startnet.cmd + winpeshl.ini in %s", wimPath)
	return nil
}

func (m *Manager) AddDirectory(wimPath string, index int, source, target string) error {
	quote := func(value string) string { return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"` }
	cmd := exec.Command(m.wimlibPath, "update", wimPath, strconv.Itoa(index))
	cmd.Stdin = strings.NewReader("add " + quote(source) + " " + quote(target) + "\n")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot add drivers to WIM image %d: %w\n%s", index, err, output)
	}
	return nil
}

func writeTempCRLF(pattern, content string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(strings.ReplaceAll(content, "\n", "\r\n")); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

func (m *Manager) OptimiseWIM(wimPath string) error {
	log.Printf("Optimising WIM file: %s", wimPath)
	cmd := exec.Command(m.wimlibPath, "optimize", wimPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to optimise WIM: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, input, 0644); err != nil {
		return err
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}
