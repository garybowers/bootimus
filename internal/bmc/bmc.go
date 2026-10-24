package bmc

import (
	"context"
	"fmt"
	"sort"

	"bootimus/internal/models"
)

type Action string

const (
	PowerOn               Action = "On"
	PowerOff              Action = "ForceOff"
	PowerRestart          Action = "ForceRestart"
	PowerGracefulShutdown Action = "GracefulShutdown"
	PowerGracefulRestart  Action = "GracefulRestart"
)

const DefaultProtocol = "redfish"

type Target struct {
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
	Insecure bool
}

type Controller interface {
	PowerState(ctx context.Context) (string, error)
	SetPower(ctx context.Context, action Action) error
}

type Protocol interface {
	Name() string
	New(t Target) (Controller, error)
}

var protocols = map[string]Protocol{}

func RegisterProtocol(p Protocol) {
	protocols[p.Name()] = p
}

func ProtocolNames() []string {
	names := make([]string, 0, len(protocols))
	for name := range protocols {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func New(t Target) (Controller, error) {
	p, ok := protocols[t.Protocol]
	if !ok {
		return nil, fmt.Errorf("unknown BMC protocol %q", t.Protocol)
	}
	return p.New(t)
}

func Resolve(c *models.Client, g *models.ClientGroup) (Target, bool) {
	t := Target{
		Protocol: c.BMCProtocol,
		Host:     c.IPMIHost,
		Port:     c.IPMIPort,
		Username: c.IPMIUsername,
		Password: c.IPMIPassword,
		Insecure: c.IPMIInsecure,
	}
	if g != nil {
		if t.Protocol == "" {
			t.Protocol = g.BMCProtocol
		}
		if t.Port == 0 {
			t.Port = g.IPMIPort
		}
		if t.Username == "" {
			t.Username = g.IPMIUsername
		}
		if t.Password == "" {
			t.Password = g.IPMIPassword
		}
		if !t.Insecure {
			t.Insecure = g.IPMIInsecure
		}
	}
	if t.Protocol == "" {
		t.Protocol = DefaultProtocol
	}
	if t.Host == "" || t.Username == "" || t.Password == "" {
		return Target{}, false
	}
	return t, true
}
