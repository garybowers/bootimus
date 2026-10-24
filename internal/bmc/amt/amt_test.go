package amt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"bootimus/internal/bmc"
)

const powerServiceResponse = `<?xml version="1.0" encoding="UTF-8"?>
<a:Envelope xmlns:a="http://www.w3.org/2003/05/soap-envelope" xmlns:b="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:c="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:g="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_AssociatedPowerManagementService">
  <a:Header>
    <b:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</b:To>
    <b:RelatesTo>0</b:RelatesTo>
    <b:Action a:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/09/transfer/GetResponse</b:Action>
    <b:MessageID>uuid:00000000-8086-8086-8086-000000007A93</b:MessageID>
    <c:ResourceURI>http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_AssociatedPowerManagementService</c:ResourceURI>
  </a:Header>
  <a:Body>
    <g:CIM_AssociatedPowerManagementService>
%s
      <g:PowerState>%d</g:PowerState>
    </g:CIM_AssociatedPowerManagementService>
  </a:Body>
</a:Envelope>`

const powerChangeResponse = `<?xml version="1.0" encoding="UTF-8"?>
<a:Envelope xmlns:a="http://www.w3.org/2003/05/soap-envelope" xmlns:b="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:c="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:g="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_PowerManagementService">
  <a:Header>
    <b:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</b:To>
    <b:RelatesTo>0</b:RelatesTo>
    <b:Action a:mustUnderstand="true">http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_PowerManagementService/RequestPowerStateChangeResponse</b:Action>
    <b:MessageID>uuid:00000000-8086-8086-8086-0000000003B8</b:MessageID>
    <c:ResourceURI>http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_PowerManagementService</c:ResourceURI>
  </a:Header>
  <a:Body>
    <g:RequestPowerStateChange_OUTPUT>
      <g:ReturnValue>%d</g:ReturnValue>
    </g:RequestPowerStateChange_OUTPUT>
  </a:Body>
</a:Envelope>`

type fakeAMT struct {
	mu          sync.Mutex
	powerState  int
	available   []int
	returnValue int
	requested   []int
	authHeaders []string
}

func (f *fakeAMT) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wsman" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="Digest:0123456789ABCDEF", nonce="abcdef", stale="false", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.authHeaders = append(f.authHeaders, auth)
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		switch {
		case strings.Contains(string(body), "RequestPowerStateChange_INPUT"):
			var state int
			fmt.Sscanf(string(body[strings.Index(string(body), "<h:PowerState>")+len("<h:PowerState>"):]), "%d", &state)
			f.requested = append(f.requested, state)
			fmt.Fprintf(w, powerChangeResponse, f.returnValue)
		case strings.Contains(string(body), "CIM_AssociatedPowerManagementService"):
			var avail strings.Builder
			for _, s := range f.available {
				fmt.Fprintf(&avail, "      <g:AvailableRequestedPowerStates>%d</g:AvailableRequestedPowerStates>\n", s)
			}
			fmt.Fprintf(w, powerServiceResponse, avail.String(), f.powerState)
		default:
			t.Errorf("unexpected WS-MAN request: %s", body)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}
}

type redirectTransport struct {
	target *url.URL
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestClient(t *testing.T, fake *fakeAMT) *Client {
	t.Helper()
	ts := httptest.NewServer(fake.handler(t))
	t.Cleanup(ts.Close)
	u, _ := url.Parse(ts.URL)
	c, err := New("amt.example", 0, "admin", "secret", false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.transport = redirectTransport{target: u}
	return c
}

func TestNewPortSelection(t *testing.T) {
	for _, tc := range []struct {
		port   int
		useTLS bool
		ok     bool
	}{
		{0, false, true},
		{16992, false, true},
		{16993, true, true},
		{443, false, false},
	} {
		c, err := New("h", tc.port, "u", "p", false)
		if tc.ok != (err == nil) {
			t.Errorf("port %d: err = %v", tc.port, err)
			continue
		}
		if tc.ok && c.UseTLS != tc.useTLS {
			t.Errorf("port %d: UseTLS = %v, want %v", tc.port, c.UseTLS, tc.useTLS)
		}
	}
}

func TestPowerStateUsesDigestAuth(t *testing.T) {
	fake := &fakeAMT{powerState: 2}
	c := newTestClient(t, fake)

	state, err := c.PowerState(context.Background())
	if err != nil {
		t.Fatalf("PowerState: %v", err)
	}
	if state != "On" {
		t.Fatalf("state = %q, want On", state)
	}
	if len(fake.authHeaders) != 1 || !strings.HasPrefix(fake.authHeaders[0], "Digest ") {
		t.Fatalf("expected one digest-authenticated request, got %v", fake.authHeaders)
	}
}

func TestPowerStateNames(t *testing.T) {
	for state, want := range map[int]string{2: "On", 3: "Sleep", 7: "Hibernate", 8: "Off", 6: "Off", 1: "Unknown (1)"} {
		fake := &fakeAMT{powerState: state}
		got, err := newTestClient(t, fake).PowerState(context.Background())
		if err != nil {
			t.Fatalf("PowerState(%d): %v", state, err)
		}
		if got != want {
			t.Errorf("PowerState(%d) = %q, want %q", state, got, want)
		}
	}
}

func TestSetPowerMapsActions(t *testing.T) {
	for action, want := range map[bmc.Action]int{
		bmc.PowerOn:               2,
		bmc.PowerOff:              8,
		bmc.PowerRestart:          10,
		bmc.PowerGracefulShutdown: 12,
		bmc.PowerGracefulRestart:  14,
	} {
		fake := &fakeAMT{powerState: 2, available: []int{2, 8, 10, 12, 14}}
		if err := newTestClient(t, fake).SetPower(context.Background(), action); err != nil {
			t.Fatalf("SetPower(%s): %v", action, err)
		}
		if len(fake.requested) != 1 || fake.requested[0] != want {
			t.Errorf("SetPower(%s) requested %v, want [%d]", action, fake.requested, want)
		}
	}
}

func TestSetPowerRejectsUnavailableState(t *testing.T) {
	fake := &fakeAMT{powerState: 8, available: []int{2}}
	err := newTestClient(t, fake).SetPower(context.Background(), bmc.PowerGracefulShutdown)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Intel LMS") {
		t.Fatalf("error should explain the LMS requirement, got: %v", err)
	}
	if len(fake.requested) != 0 {
		t.Fatalf("no power change should have been sent, got %v", fake.requested)
	}
}

func TestSetPowerRejectsUnknownAction(t *testing.T) {
	fake := &fakeAMT{powerState: 2, available: []int{2, 8}}
	if err := newTestClient(t, fake).SetPower(context.Background(), bmc.Action("Nmi")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSetPowerReportsReturnValue(t *testing.T) {
	fake := &fakeAMT{powerState: 2, available: []int{2, 8}, returnValue: 2}
	err := newTestClient(t, fake).SetPower(context.Background(), bmc.PowerOff)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "UnknownError") {
		t.Fatalf("error should name the return value, got: %v", err)
	}
}

func TestPowerStateHonoursContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(ts.Close)
	u, _ := url.Parse(ts.URL)
	c, _ := New("amt.example", 0, "admin", "secret", false)
	c.transport = redirectTransport{target: u}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.PowerState(ctx)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("PowerState took %s to give up", time.Since(start))
	}
}
