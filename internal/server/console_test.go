package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bootimus/internal/bmc"
	"bootimus/internal/models"

	"github.com/gorilla/websocket"
)

type fakeConsole struct {
	mu     sync.Mutex
	out    chan []byte
	closed chan struct{}
	once   sync.Once
	wrote  [][]byte
}

func newFakeConsole() *fakeConsole {
	return &fakeConsole{out: make(chan []byte, 8), closed: make(chan struct{})}
}

func (f *fakeConsole) Read(p []byte) (int, error) {
	select {
	case data := <-f.out:
		return copy(p, data), nil
	case <-f.closed:
		return 0, io.EOF
	}
}

func (f *fakeConsole) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.wrote = append(f.wrote, append([]byte(nil), p...))
	f.mu.Unlock()
	f.out <- []byte(strings.ToUpper(string(p)))
	return len(p), nil
}

func (f *fakeConsole) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func consoleEnv(t *testing.T) (*accessEnv, *httptest.Server) {
	t.Helper()
	env := newAccessEnv(t)
	client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:10", Name: "bmc-box", Static: true, Enabled: true,
		BMCProtocol: "amt", IPMIHost: "10.0.0.50", IPMIUsername: "admin", IPMIPassword: "secret"}
	if err := env.store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	plain := &models.Client{MACAddress: "aa:bb:cc:dd:ee:11", Name: "no-bmc", Static: true, Enabled: true}
	if err := env.store.CreateClient(plain); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(env.handler)
	t.Cleanup(srv.Close)
	return env, srv
}

func TestConsoleRejectsUnauthenticatedAndNonAdmin(t *testing.T) {
	env, _ := consoleEnv(t)
	if code, _ := env.do(http.MethodGet, "/api/clients/console?mac=aa:bb:cc:dd:ee:10", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/console?mac=aa:bb:cc:dd:ee:10", env.viewer); code != http.StatusForbidden {
		t.Fatalf("viewer: %d", code)
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/console?mac=00:00:00:00:00:00", env.admin); code != http.StatusNotFound {
		t.Fatalf("unknown mac: %d", code)
	}
	if code, body := env.do(http.MethodGet, "/api/clients/console?mac=aa:bb:cc:dd:ee:11", env.admin); code != http.StatusBadRequest {
		t.Fatalf("client without BMC: %d %s", code, body)
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/console?mac=aa:bb:cc:dd:ee:10&token=not-a-jwt", ""); code != http.StatusUnauthorized {
		t.Fatalf("bad query token: %d", code)
	}
}

func TestConsoleStreamsOverWebSocket(t *testing.T) {
	env, srv := consoleEnv(t)
	fake := newFakeConsole()
	var gotTarget bmc.Target
	env.server.openConsole = func(ctx context.Context, target bmc.Target) (bmc.Console, error) {
		gotTarget = target
		return fake, nil
	}

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/clients/console?mac=AA-BB-CC-DD-EE-10"
	dialer := websocket.Dialer{Subprotocols: []string{"bearer", env.admin}, HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v (resp %v)", err, resp)
	}
	defer conn.Close()
	if resp.Header.Get("Sec-WebSocket-Protocol") != "bearer" {
		t.Fatalf("negotiated subprotocol %q", resp.Header.Get("Sec-WebSocket-Protocol"))
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	mt, msg, err := conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage || !strings.Contains(string(msg), `"connected"`) {
		t.Fatalf("first message: %d %q %v", mt, msg, err)
	}
	if gotTarget.Host != "10.0.0.50" || gotTarget.Protocol != "amt" {
		t.Fatalf("target = %+v", gotTarget)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	mt, msg, err = conn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || string(msg) != "LS\r" {
		t.Fatalf("echo: %d %q %v", mt, msg, err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":80,"rows":24}`)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("x")); err != nil {
		t.Fatal(err)
	}
	mt, msg, err = conn.ReadMessage()
	if err != nil || string(msg) != "X" {
		t.Fatalf("text keystroke echo: %d %q %v", mt, msg, err)
	}

	second := websocket.Dialer{Subprotocols: []string{"bearer", env.admin}, HandshakeTimeout: 5 * time.Second}
	if _, resp2, err := second.Dial(url, nil); err == nil || resp2 == nil || resp2.StatusCode != http.StatusConflict {
		code := 0
		if resp2 != nil {
			code = resp2.StatusCode
		}
		t.Fatalf("second session: err=%v code=%d, want 409", err, code)
	}

	fake.Close()
	mt, msg, err = conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage || !strings.Contains(string(msg), `"closed"`) {
		t.Fatalf("close notice: %d %q %v", mt, msg, err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected the socket to close after the console ended")
	}
	deadline := time.Now().Add(3 * time.Second)
	sessionActive := func() bool {
		env.server.consoles.mu.Lock()
		defer env.server.consoles.mu.Unlock()
		return env.server.consoles.active["aa:bb:cc:dd:ee:10"]
	}
	for sessionActive() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sessionActive() {
		t.Fatal("session was not released")
	}
}

func TestConsoleReportsOpenFailure(t *testing.T) {
	env, srv := consoleEnv(t)
	env.server.openConsole = func(ctx context.Context, target bmc.Target) (bmc.Console, error) {
		return nil, bmc.ErrConsoleUnsupported
	}
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/clients/console?mac=aa:bb:cc:dd:ee:10&token=" + env.admin
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	mt, msg, err := conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage || !strings.Contains(string(msg), "no serial console support") {
		t.Fatalf("error notice: %d %q %v", mt, msg, err)
	}
}

func TestConsoleTokenSources(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/clients/console?mac=x", nil)
	r.Header.Add("Sec-WebSocket-Protocol", "bearer, abc.def.ghi")
	if got := consoleToken(r); got != "abc.def.ghi" {
		t.Fatalf("subprotocol token = %q", got)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/clients/console?mac=x&token=qqq", nil)
	if got := consoleToken(r); got != "qqq" {
		t.Fatalf("query token = %q", got)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/clients/console?mac=x", nil)
	r.Header.Set("Authorization", "Bearer hhh")
	if got := consoleToken(r); got != "hhh" {
		t.Fatalf("header token = %q", got)
	}
}
