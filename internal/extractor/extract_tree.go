package extractor

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
)

func ExtractTree(isoPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	err := extractTreeViaISO9660(isoPath, destDir)
	if err == nil {
		return nil
	}

	log.Printf("ISO9660 tree extraction failed (%v), trying bsdtar", err)
	bsdtarPath, lookErr := exec.LookPath("bsdtar")
	if lookErr != nil {
		return fmt.Errorf("iso9660 tree extraction failed and bsdtar not available: %w", err)
	}
	if output, tarErr := exec.Command(bsdtarPath, "-xf", isoPath, "-C", destDir).CombinedOutput(); tarErr != nil {
		return fmt.Errorf("bsdtar failed: %w (%s)", tarErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func extractTreeViaISO9660(isoPath, destDir string) error {
	f, err := os.Open(isoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return err
	}
	root, err := img.RootDir()
	if err != nil {
		return err
	}
	return extractTreeNode(root, destDir, destDir)
}

func extractTreeNode(node *iso9660.File, target, rootDir string) error {
	if target != rootDir && !strings.HasPrefix(filepath.Clean(target), filepath.Clean(rootDir)+string(os.PathSeparator)) {
		return nil
	}

	if node.IsDir() {
		if err := os.MkdirAll(target, 0755); err != nil {
			return err
		}
		children, err := safeGetChildren(node)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := extractTreeNode(child, filepath.Join(target, child.Name()), rootDir); err != nil {
				return err
			}
		}
		return nil
	}

	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, node.Reader())
	return err
}
