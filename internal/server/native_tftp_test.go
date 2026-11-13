package server

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bootimus/internal/models"
)

type nativeTFTPTransfer struct {
	tftpCapture
	fail bool
}

func (r *nativeTFTPTransfer) RemoteAddr() net.UDPAddr {
	return net.UDPAddr{IP: net.ParseIP("192.0.2.54"), Port: 3456}
}
func (r *nativeTFTPTransfer) ReadFrom(src io.Reader) (int64, error) {
	if r.fail {
		return 0, errors.New("transfer interrupted")
	}
	return r.tftpCapture.ReadFrom(src)
}

func TestNativePiTFTPTransferStatistics(t *testing.T) {
	for _, source := range []string{"custom set", "raspberrypi override"} {
		t.Run(source, func(t *testing.T) {
			s, store := newBootloaderSelectionTestServer(t)
			s.SetActiveBootloaderSet("rpi-classic")
			client := &models.Client{MACAddress: "b8:27:eb:84:08:3c", Static: true, NativeTFTPBootFile: "ec84083c/kernel7.img"}
			if err := store.CreateClient(client); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(s.config.BootDir, "rpi-classic")
			if source == "raspberrypi override" {
				s.config.DataDir = t.TempDir()
				base = filepath.Join(s.config.DataDir, "raspberrypi")
			}
			if err := os.MkdirAll(filepath.Join(base, "ec84083c"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"config.txt", "cmdline.txt", "kernel7.img"} {
				if err := os.WriteFile(filepath.Join(base, "ec84083c", name), []byte("fixture"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"config.txt", "cmdline.txt"} {
				if err := s.serveTFTPFile("ec84083c/"+name, &nativeTFTPTransfer{}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.serveTFTPFile(client.NativeTFTPBootFile, &nativeTFTPTransfer{fail: true}); err == nil {
				t.Fatal("expected transfer failure")
			}
			got, _ := store.GetClient(client.MACAddress)
			if got.BootCount != 0 || got.LastBoot != nil {
				t.Fatal("counted discovery or failed transfer")
			}
			for range 3 {
				if err := s.serveTFTPFile(client.NativeTFTPBootFile, &nativeTFTPTransfer{}); err != nil {
					t.Fatal(err)
				}
			}
			got, _ = store.GetClient(client.MACAddress)
			if got.BootCount != 1 || got.LastBoot == nil {
				t.Fatalf("statistics: %+v", got)
			}
			entries, err := store.GetBootLogsByMAC(client.MACAddress, 10)
			if err != nil || len(entries) != 1 || entries[0].IPAddress != "192.0.2.54" {
				t.Fatalf("activity: %+v, %v", entries, err)
			}
		})
	}
}

func TestNativeTFTPTimelineDescribesTransfer(t *testing.T) {
	item := bootLogItem(models.BootLog{Source: models.BootSourceNativeTFTP, ImageName: "Native TFTP transfer: ec84083c/kernel7.img", Success: true})
	if item.Title != "Native TFTP transfer: ec84083c/kernel7.img" || !strings.Contains(item.Detail, "OS startup not confirmed") {
		t.Fatalf("misleading timeline entry: %+v", item)
	}
}
