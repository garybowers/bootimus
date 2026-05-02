package wim

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	cmd := exec.Command(m.wimlibPath, "update", wimPath, "2", "--rebuild")
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wimlib-imagex update failed: %w\nOutput: %s", err, string(output))
	}

	log.Printf("WIM: patched startnet.cmd + winpeshl.ini in %s", wimPath)
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

func (m *Manager) OptimizeWIM(wimPath string) error {
	log.Printf("Optimizing WIM file: %s", wimPath)
	cmd := exec.Command(m.wimlibPath, "optimize", wimPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to optimize WIM: %w\nOutput: %s", err, string(output))
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
