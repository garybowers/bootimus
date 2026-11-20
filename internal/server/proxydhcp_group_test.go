package server

import (
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestProxyDHCPGroupVendorOption(t *testing.T) {
	store := storagetest.New(t)
	server := &Server{config: &Config{Storage: store}}
	group := &models.ClientGroup{Name: "lab", Enabled: true, ProxyDHCPNoVendorOption: true}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	member := &models.Client{MACAddress: "02:00:00:00:00:01", ClientGroupID: &group.ID}
	individual := &models.Client{MACAddress: "02:00:00:00:00:02", ClientGroupID: &group.ID, ProxyDHCPNoVendorOption: true}
	other := &models.Client{MACAddress: "02:00:00:00:00:03"}
	for _, client := range []*models.Client{member, individual, other} {
		if err := store.CreateClient(client); err != nil {
			t.Fatal(err)
		}
	}

	check := func(mac string, omit bool) {
		t.Helper()
		if got := server.proxyDHCPOmitVendorOption(mustParseMAC(t, mac)); got != omit {
			t.Fatalf("omit option 43 for %s = %v, want %v (group enabled=%v, omit=%v)", mac, got, omit, group.Enabled, group.ProxyDHCPNoVendorOption)
		}
	}
	check(member.MACAddress, true)
	check(other.MACAddress, false)
	check("02:00:00:00:00:ff", false)
	if err := store.SetClientGroup(other.MACAddress, &group.ID); err != nil {
		t.Fatal(err)
	}
	check(other.MACAddress, true)
	if err := store.SetClientGroup(member.MACAddress, nil); err != nil {
		t.Fatal(err)
	}
	check(member.MACAddress, false)

	for _, flags := range []struct{ enabled, omit bool }{{false, true}, {true, false}, {true, true}} {
		group.Enabled, group.ProxyDHCPNoVendorOption = flags.enabled, flags.omit
		if err := store.UpdateClientGroup(group.ID, group); err != nil {
			t.Fatal(err)
		}
		check(other.MACAddress, flags.enabled && flags.omit)
		check(individual.MACAddress, true)
	}
	if err := store.DeleteClientGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	check(other.MACAddress, false)
	check(individual.MACAddress, true)
	recreated := &models.ClientGroup{Name: group.Name, Enabled: true}
	if err := store.CreateClientGroup(recreated); err != nil {
		t.Fatal(err)
	}
	if err := store.SetClientGroup(other.MACAddress, &recreated.ID); err != nil {
		t.Fatal(err)
	}
	check(other.MACAddress, false)
}
