package bmc

import (
	"testing"

	"bootimus/internal/models"
)

func TestResolveInheritsFromGroup(t *testing.T) {
	c := &models.Client{IPMIHost: "10.0.0.5"}
	g := &models.ClientGroup{BMCProtocol: "amt", IPMIPort: 16993, IPMIUsername: "admin", IPMIPassword: "secret", IPMIInsecure: true}

	target, ok := Resolve(c, g)
	if !ok {
		t.Fatal("expected a resolved target")
	}
	want := Target{Protocol: "amt", Host: "10.0.0.5", Port: 16993, Username: "admin", Password: "secret", Insecure: true}
	if target != want {
		t.Fatalf("got %+v, want %+v", target, want)
	}
}

func TestResolveClientOverridesGroup(t *testing.T) {
	c := &models.Client{BMCProtocol: "redfish", IPMIHost: "10.0.0.5", IPMIPort: 8443, IPMIUsername: "root", IPMIPassword: "pw"}
	g := &models.ClientGroup{BMCProtocol: "amt", IPMIPort: 16992, IPMIUsername: "admin", IPMIPassword: "secret"}

	target, ok := Resolve(c, g)
	if !ok {
		t.Fatal("expected a resolved target")
	}
	if target.Protocol != "redfish" || target.Port != 8443 || target.Username != "root" || target.Password != "pw" {
		t.Fatalf("client values were not preferred: %+v", target)
	}
}

func TestResolveDefaultsToRedfish(t *testing.T) {
	c := &models.Client{IPMIHost: "10.0.0.5", IPMIUsername: "root", IPMIPassword: "pw"}

	target, ok := Resolve(c, nil)
	if !ok {
		t.Fatal("expected a resolved target")
	}
	if target.Protocol != DefaultProtocol {
		t.Fatalf("protocol = %q, want %q", target.Protocol, DefaultProtocol)
	}
}

func TestResolveRequiresHostAndCredentials(t *testing.T) {
	cases := map[string]*models.Client{
		"no host":     {IPMIUsername: "root", IPMIPassword: "pw"},
		"no user":     {IPMIHost: "10.0.0.5", IPMIPassword: "pw"},
		"no password": {IPMIHost: "10.0.0.5", IPMIUsername: "root"},
	}
	for name, c := range cases {
		if _, ok := Resolve(c, nil); ok {
			t.Errorf("%s: expected unresolved", name)
		}
	}
}

func TestNewRejectsUnknownProtocol(t *testing.T) {
	if _, err := New(Target{Protocol: "ipmi-over-carrier-pigeon"}); err == nil {
		t.Fatal("expected an error for an unregistered protocol")
	}
}
