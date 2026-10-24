package amt

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"bootimus/internal/bmc"

	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/associatedpower"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/models"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/client"
)

const Name = "amt"

const (
	HTTPPort = 16992
	TLSPort  = 16993
)

var actionStates = map[bmc.Action]models.PowerState{
	bmc.PowerOn:               models.PowerStateOn,
	bmc.PowerOff:              models.PowerStateOffSoft,
	bmc.PowerRestart:          models.PowerStateMasterBusReset,
	bmc.PowerGracefulShutdown: models.PowerStatePowerOffSoftGraceful,
	bmc.PowerGracefulRestart:  models.PowerStateMasterBusResetGraceful,
}

type Protocol struct{}

func init() {
	bmc.RegisterProtocol(Protocol{})
}

func (Protocol) Name() string {
	return Name
}

func (Protocol) New(t bmc.Target) (bmc.Controller, error) {
	return New(t.Host, t.Port, t.Username, t.Password, t.Insecure)
}

type Client struct {
	Host      string
	UseTLS    bool
	Username  string
	Password  string
	Insecure  bool
	transport http.RoundTripper
}

func New(host string, port int, username, password string, insecure bool) (*Client, error) {
	c := &Client{Host: host, Username: username, Password: password, Insecure: insecure}
	switch port {
	case 0, HTTPPort:
	case TLSPort:
		c.UseTLS = true
	default:
		return nil, fmt.Errorf("Intel AMT listens on port %d, or %d with TLS, not %d", HTTPPort, TLSPort, port)
	}
	return c, nil
}

func (c *Client) PowerState(ctx context.Context) (string, error) {
	m := c.messages()
	defer closeIdle(m)
	svc, err := getPowerService(ctx, m)
	if err != nil {
		return "", err
	}
	return stateName(svc.PowerState), nil
}

func (c *Client) SetPower(ctx context.Context, action bmc.Action) error {
	want, ok := actionStates[action]
	if !ok {
		return fmt.Errorf("Intel AMT does not support the %s power action", action)
	}
	m := c.messages()
	defer closeIdle(m)
	svc, err := getPowerService(ctx, m)
	if err != nil {
		return err
	}
	if !slices.Contains(svc.AvailableRequestedPowerStates, models.AvailableRequestedPowerStates(want)) {
		if action == bmc.PowerGracefulShutdown || action == bmc.PowerGracefulRestart {
			return fmt.Errorf("Intel AMT does not offer %s while the machine is %s; graceful actions need Intel LMS running in the operating system", action, stateName(svc.PowerState))
		}
		return fmt.Errorf("Intel AMT does not offer %s while the machine is %s", action, stateName(svc.PowerState))
	}
	resp, err := withContext(ctx, func() (power.Response, error) {
		return m.CIM.PowerManagementService.RequestPowerStateChange(power.PowerState(want))
	})
	if err != nil {
		return err
	}
	if rv := resp.Body.RequestPowerStateChangeResponse.ReturnValue; rv != power.ReturnValueCompletedWithNoError {
		return fmt.Errorf("Intel AMT rejected %s: %s", action, rv)
	}
	return nil
}

func (c *Client) messages() wsman.Messages {
	return wsman.NewMessages(client.Parameters{
		Target:            c.Host,
		Username:          c.Username,
		Password:          c.Password,
		UseDigest:         true,
		UseTLS:            c.UseTLS,
		SelfSignedAllowed: c.Insecure,
		Transport:         c.transport,
		Timeout:           30 * time.Second,
	})
}

func closeIdle(m wsman.Messages) {
	if t, ok := m.Client.(*client.Target); ok {
		t.CloseIdleConnections()
	}
}

func getPowerService(ctx context.Context, m wsman.Messages) (associatedpower.CIM_AssociatedPowerManagementService, error) {
	resp, err := withContext(ctx, m.CIM.AssociatedPowerManagementService.Get)
	if err != nil {
		return associatedpower.CIM_AssociatedPowerManagementService{}, err
	}
	return resp.Body.AssociatedPowerManagementService, nil
}

func stateName(s models.PowerState) string {
	switch s {
	case models.PowerStateOn:
		return "On"
	case models.PowerStateSleepLight, models.PowerStateSleepDeep:
		return "Sleep"
	case models.PowerStateHibernate:
		return "Hibernate"
	case models.PowerStateOffHard, models.PowerStateOffSoft, models.PowerStatePowerOffSoftGraceful, models.PowerStatePowerOffHardGraceful:
		return "Off"
	default:
		return fmt.Sprintf("Unknown (%d)", int(s))
	}
}

func withContext[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
