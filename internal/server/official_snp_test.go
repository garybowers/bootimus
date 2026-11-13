package server

import (
	"bytes"
	"testing"

	"bootimus/bootloaders"
	"bootimus/internal/models"
)

func TestOfficialSNPSelectionAndTFTP(t *testing.T) {
	s, store := newBootloaderSelectionTestServer(t)
	const set = "secureboot-official-snp"
	group := &models.ClientGroup{Name: "XCP-ng", BootloaderSet: set}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	client := &models.Client{MACAddress: "02:00:00:00:55:01", ClientGroupID: &group.ID}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	_, uefi, arm64 := s.proxyDHCPBootfilesForClient(mustParseMAC(t, client.MACAddress))
	if uefi != "bootloader-sets/"+set+"/ipxe-shimx64.efi" || arm64 != "bootloader-sets/"+set+"/ipxe-arm64-shim.efi" {
		t.Fatalf("group bootfiles = %q, %q", uefi, arm64)
	}
	for _, name := range []string{"ipxe-shimx64.efi", "ipxe.efi", "ipxe-arm64-shim.efi", "ipxe-arm64.efi"} {
		capture := &tftpCapture{}
		if err := s.serveTFTPFile("bootloader-sets/"+set+"/"+name, capture); err != nil {
			t.Fatal(err)
		}
		want, _, err := bootloaders.Resolve(set, name)
		if err != nil || !bytes.Equal(capture.Bytes(), want) {
			t.Fatalf("wrong qualified payload for %s", name)
		}
	}
	// Firmware without a loaded-image pathname falls back to the global set.
	s.SetActiveBootloaderSet(set)
	capture := &tftpCapture{}
	if err := s.serveTFTPFile("/ipxe.efi", capture); err != nil {
		t.Fatal(err)
	}
	want, _, _ := bootloaders.Resolve(set, "ipxe.efi")
	if !bytes.Equal(capture.Bytes(), want) {
		t.Fatal("root fallback did not serve SNP loader")
	}
}
