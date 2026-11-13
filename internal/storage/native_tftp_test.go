package storage

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"bootimus/internal/models"
)

func TestNativeTFTPBootAccounting(t *testing.T) {
	store := newTestStore(t)
	client := &models.Client{MACAddress: "b8:27:eb:84:08:3c", NativeTFTPBootFile: "ec84083c/kernel7.img", Static: true}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		file   string
		offset time.Duration
		want   bool
	}{
		{"ec84083c/config.txt", 0, false},
		{"ec84083c/cmdline.txt", 0, false},
		{"ec84083c/kernel7.img", 0, true},
		{"ec84083c/kernel7.img", time.Second, false},
		{"ec84083c/kernel7.img", 29 * time.Second, false},
		{"ec84083c/kernel7.img", 30 * time.Second, true},
		{"another/kernel7.img", 31 * time.Second, false},
	} {
		got, err := recordNativeTFTPBoot(store.db, tc.file, "[2001:db8::1]:1234", now.Add(tc.offset))
		if err != nil || got != tc.want {
			t.Fatalf("%s at %s: %v, %v", tc.file, tc.offset, got, err)
		}
	}
	got, err := store.GetClient(client.MACAddress)
	if err != nil {
		t.Fatal(err)
	}
	if got.BootCount != 2 || got.LastBoot == nil || !got.LastBoot.Equal(now.Add(30*time.Second)) || !got.Static {
		t.Fatalf("unexpected statistics: %+v", got)
	}
	entries, err := store.GetBootLogsByMAC(client.MACAddress, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("logs = %d, %v", len(entries), err)
	}
	for _, entry := range entries {
		if entry.ClientID == nil || *entry.ClientID != client.ID || entry.ImageID != nil ||
			entry.ImageName != "Native TFTP transfer: ec84083c/kernel7.img" || entry.IPAddress != "2001:db8::1" || !entry.Success {
			t.Fatalf("wrong activity: %+v", entry)
		}
	}
	// An unrelated admin save cannot clear the persisted deduplication marker.
	got.NativeTFTPLastBoot = nil
	if err := store.UpdateClient(got.MACAddress, got); err != nil {
		t.Fatal(err)
	}
	recorded, err := recordNativeTFTPBoot(store.db, client.NativeTFTPBootFile, "192.0.2.1:12", now.Add(31*time.Second))
	if err != nil || recorded {
		t.Fatalf("retry after admin update: %v, %v", recorded, err)
	}
}

func TestNativeTFTPConcurrentTransfers(t *testing.T) {
	store := newTestStore(t)
	client := &models.Client{MACAddress: "b8:27:eb:84:08:3c", NativeTFTPBootFile: "ec84083c/kernel7.img"}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	results := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.RecordNativeTFTPBoot(client.NativeTFTPBootFile, "192.0.2.2:1234")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.GetClient(client.MACAddress)
	if err != nil || got.BootCount != 1 {
		t.Fatalf("client = %+v, %v", got, err)
	}
	entries, err := store.GetBootLogsByMAC(client.MACAddress, 20)
	if err != nil || len(entries) != 1 {
		t.Fatalf("logs = %d, %v", len(entries), err)
	}
}

func TestNativeTFTPFileAssignment(t *testing.T) {
	store := newTestStore(t)
	first := &models.Client{MACAddress: "02:00:00:00:54:01", NativeTFTPBootFile: "device/custom-kernel.bin"}
	if err := store.CreateClient(first); err != nil {
		t.Fatal(err)
	}
	duplicate := &models.Client{MACAddress: "02:00:00:00:54:02", NativeTFTPBootFile: first.NativeTFTPBootFile}
	if err := store.CreateClient(duplicate); !errors.Is(err, ErrNativeTFTPBootFileExists) {
		t.Fatalf("duplicate = %v", err)
	}
	for i := 3; i <= 4; i++ {
		if err := store.CreateClient(&models.Client{MACAddress: fmt.Sprintf("02:00:00:00:54:%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	second, err := store.GetClient("02:00:00:00:54:03")
	if err != nil {
		t.Fatal(err)
	}
	second.NativeTFTPBootFile = first.NativeTFTPBootFile
	if err := store.UpdateClient(second.MACAddress, second); !errors.Is(err, ErrNativeTFTPBootFileExists) {
		t.Fatalf("duplicate update = %v", err)
	}
	first.NativeTFTPBootFile = ""
	if err := store.UpdateClient(first.MACAddress, first); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateClient(second.MACAddress, second); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteClient(second.MACAddress); err != nil {
		t.Fatal(err)
	}
	first.NativeTFTPBootFile = second.NativeTFTPBootFile
	if err := store.UpdateClient(first.MACAddress, first); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTFTPLogFailureRollsBackCount(t *testing.T) {
	store := newTestStore(t)
	client := &models.Client{MACAddress: "02:00:00:00:54:05", NativeTFTPBootFile: "device/kernel.img"}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("ALTER TABLE boot_logs RENAME TO unavailable_boot_logs").Error; err != nil {
		t.Fatal(err)
	}
	recorded, err := store.RecordNativeTFTPBoot(client.NativeTFTPBootFile, "192.0.2.2")
	if err == nil || recorded {
		t.Fatalf("missing log table: %v, %v", recorded, err)
	}
	got, err := store.GetClient(client.MACAddress)
	if err != nil || got.BootCount != 0 || got.LastBoot != nil || got.NativeTFTPLastBoot != nil {
		t.Fatalf("partial write: %+v, %v", got, err)
	}
}
