package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	alpineVersion   = "3.22.1"
	alpineBranch    = "v3.22"
	alpineISO       = "alpine-virt-" + alpineVersion + "-x86_64.iso"
	alpineISOSHA    = "42918974513750a6923393f3074c3bb226badfce4a0d0f35f90377fd789fda1f"
	alpineInitramfs = "initramfs-virt"
	alpineInitrdSHA = "385418a99b17c0ef44947d88ea448d43b2df4a2c3343da72e724c6cf3b05e9f7"
	alpineMirror    = "https://dl-cdn.alpinelinux.org/alpine/" + alpineBranch
	alpineRelease   = alpineMirror + "/releases/x86_64"
	alpineNetboot   = alpineRelease + "/netboot-" + alpineVersion
	alpineOverlay   = "e2e-apkovl.tar.gz"
	alpineCacheDir  = "alpine-virt-" + alpineVersion + "-x86_64"
)

func (h *harness) cached(name, url, sha string) string {
	t := h.t
	t.Helper()
	path := filepath.Join(h.cfg.cacheDir, name)
	if sum, err := sha256File(path); err == nil && sum == sha {
		return path
	}
	t.Logf("downloading %s", url)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("download %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download %s: %s", url, resp.Status)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		t.Fatalf("download %s: %v", url, err)
	}
	f.Close()
	sum, err := sha256File(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if sum != sha {
		os.Remove(tmp)
		t.Fatalf("%s: SHA-256 %s does not match the pinned %s", url, sum, sha)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (h *harness) buildOverlay(b *bootimus) []byte {
	t := h.t
	t.Helper()
	script := strings.Join([]string{
		"#!/bin/sh",
		"MAC=$(cat /sys/class/net/eth0/address)",
		`echo "BOOTIMUS-E2E overlay running for $MAC"`,
		fmt.Sprintf(`URL=$(wget -qO- "%s/autoinstall/%s?mac=$MAC" | grep -o 'http://[^ "]*install/callback[^ "]*' | head -1)`, b.guestURL, alpineISO),
		`echo "BOOTIMUS-E2E callback $URL"`,
		`wget -qO- "${URL}&status=completed&message=e2e-overlay"`,
		"poweroff",
		"",
	}, "\n")
	interfaces := "auto lo\niface lo inet loopback\nauto eth0\niface eth0 inet dhcp\n"

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	now := time.Now()
	add := func(hdr *tar.Header, body []byte) {
		hdr.ModTime = now
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if body != nil {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, dir := range []string{"etc", "etc/local.d", "etc/runlevels", "etc/runlevels/default", "etc/network"} {
		add(&tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0o755}, nil)
	}
	add(&tar.Header{Name: "etc/local.d/bootimus.start", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(script))}, []byte(script))
	add(&tar.Header{Name: "etc/network/interfaces", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(interfaces))}, []byte(interfaces))
	add(&tar.Header{Name: "etc/runlevels/default/local", Typeflag: tar.TypeSymlink, Linkname: "/etc/init.d/local", Mode: 0o777}, nil)
	add(&tar.Header{Name: "etc/runlevels/default/networking", Typeflag: tar.TypeSymlink, Linkname: "/etc/init.d/networking", Mode: 0o777}, nil)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (h *harness) alpineStage(b *bootimus) {
	t := h.t
	t.Helper()
	b.t = t
	iso := h.cached(alpineISO, alpineRelease+"/"+alpineISO, alpineISOSHA)
	initrd := h.cached(alpineInitramfs, alpineNetboot+"/"+alpineInitramfs, alpineInitrdSHA)

	files := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(iso))))
	defer files.Close()

	b.mustAPI(http.MethodPost, "/api/images/download", map[string]string{
		"url":      files.URL + "/" + alpineISO,
		"filename": alpineISO,
		"sha256":   alpineISOSHA,
	})
	h.waitFor(h.scale(3*time.Minute), "verified ISO download", func() (bool, string) {
		out := b.api(http.MethodGet, "/api/downloads/progress?filename="+alpineISO, nil)
		data, _ := out["data"].(map[string]any)
		status := fmt.Sprint(data["status"])
		if status == "error" {
			t.Fatalf("download failed: %v", data["error"])
		}
		return status == "completed", status
	})
	image := b.mustAPI(http.MethodGet, "/api/images?filename="+alpineISO, nil)["data"].(map[string]any)
	if verified, _ := image["checksum_verified"].(bool); !verified {
		t.Errorf("downloaded image was not marked checksum_verified: %v", image["checksum"])
	}

	b.mustAPI(http.MethodPost, "/api/images/extract?filename="+alpineISO, nil)
	image = b.mustAPI(http.MethodGet, "/api/images?filename="+alpineISO, nil)["data"].(map[string]any)
	if fmt.Sprint(image["distro"]) != "alpine" {
		t.Fatalf("expected the ISO to be detected as alpine, got %v", image["distro"])
	}
	bootDir := filepath.Join(b.dataDir, "isos", alpineCacheDir)
	if err := copyFile(initrd, filepath.Join(bootDir, "initramfs-netboot")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"iso/boot/modloop-virt", "iso/apks/x86_64/APKINDEX.tar.gz"} {
		url := fmt.Sprintf("%s/boot/%s/%s", b.bootURL, alpineCacheDir, rel)
		if resp, err := http.Head(url); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s is not served from the extracted ISO tree (install bsdtar, the libarchive-tools package, so extraction keeps full file names): %v", url, err)
		}
	}

	overlay := h.buildOverlay(b)
	b.upload("/api/files/upload", "file", alpineOverlay, overlay, map[string]string{"public": "true"})
	if _, err := httpGet(b.bootURL + "/files/" + alpineOverlay); err != nil {
		t.Fatalf("overlay is not served on the boot port: %v", err)
	}

	bootParams := strings.Join([]string{
		"modules=loop,squashfs,sd-mod,usb-storage",
		"console=ttyS0,115200",
		"ip=dhcp",
		fmt.Sprintf("modloop=%s/boot/%s/iso/boot/modloop-virt", b.guestURL, alpineCacheDir),
		fmt.Sprintf("alpine_repo=%s/boot/%s/iso/apks", b.guestURL, alpineCacheDir),
		fmt.Sprintf("apkovl=%s/files/%s", b.guestURL, alpineOverlay),
	}, " ")
	b.mustAPI(http.MethodPut, "/api/images?filename="+alpineISO, map[string]any{
		"boot_params":     bootParams,
		"initrd_override": "initramfs-netboot",
	})
	b.mustAPI(http.MethodPut, "/api/images/autoinstall?filename="+alpineISO, map[string]any{
		"script":      "#cloud-config\n# {{CALLBACK_URL}}\n",
		"enabled":     true,
		"script_type": "autoinstall",
	})

	b.mustAPI(http.MethodPost, "/api/clients", map[string]any{"mac_address": macAlpine, "name": "e2e-alpine"})
	b.mustAPI(http.MethodPost, "/api/clients/next-boot", map[string]string{"mac_address": macAlpine, "image_filename": alpineISO})
	preview := b.mustAPI(http.MethodGet, "/api/clients/menu-preview?mac="+macAlpine, nil)["data"].(map[string]any)
	if fmt.Sprint(preview["next_boot_image"]) == "" {
		t.Fatalf("menu preview does not show the next-boot image: %v", preview)
	}
	script := fmt.Sprint(preview["script"])
	if !strings.Contains(script, "goto iso") || !strings.Contains(script, "initramfs-netboot") {
		t.Fatalf("menu preview does not pre-select the Alpine image with the overridden initrd:\n%s", script)
	}

	bootRequestsBefore := b.metric("bootimus_http_boot_requests_total")
	v := h.startVM(vmOptions{name: "alpine", mac: macAlpine, bootfile: loaderBIOS, tftpDir: h.stagingDir(b)})

	h.waitFor(h.scale(2*time.Minute), "Alpine kernel booting on the serial console", func() (bool, string) {
		return v.serialContains(serialLinuxBanner) && v.serialContains(serialAlpineInit), v.serialTail()
	})
	h.waitFor(h.scale(30*time.Second), "kernel and initrd fetched over HTTP", func() (bool, string) {
		now := b.metric("bootimus_http_boot_requests_total")
		return now >= bootRequestsBefore+2, fmt.Sprintf("bootimus_http_boot_requests_total %v -> %v", bootRequestsBefore, now)
	})
	h.waitFor(h.scale(4*time.Minute), "overlay to run and power the VM off", func() (bool, string) {
		return v.exited(), v.serialTail()
	})
	h.waitFor(h.scale(30*time.Second), "install record completed by the callback", func() (bool, string) {
		out := b.api(http.MethodGet, "/api/clients/installs?mac="+macAlpine, nil)
		records, _ := out["data"].([]any)
		for _, r := range records {
			rec, _ := r.(map[string]any)
			if fmt.Sprint(rec["state"]) == "completed" {
				return true, ""
			}
		}
		return false, fmt.Sprint(out)
	})
	if !b.logContains("install of " + alpineCacheDir + " completed") {
		t.Errorf("server log has no install completion line; tail:\n%s", tail(b.logPath, 20))
	}
	client := b.mustAPI(http.MethodGet, "/api/clients?mac="+macAlpine, nil)["data"].(map[string]any)
	if pending, _ := client["next_boot_image"].(string); pending != "" {
		t.Errorf("next-boot image was not cleared after use: %q", pending)
	}
	if boots, _ := client["boot_count"].(float64); boots < 1 {
		t.Errorf("the boot was not counted on the client: boot_count=%v", client["boot_count"])
	}
	logs, _ := b.mustAPI(http.MethodGet, "/api/logs?mac="+macAlpine+"&limit=10", nil)["data"].([]any)
	if len(logs) == 0 {
		t.Errorf("the boot left no entry in the client's boot history")
	}
}
