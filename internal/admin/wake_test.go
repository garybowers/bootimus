package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestWakeClientGroupReportsSendError(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("group override=%t", override), func(t *testing.T) {
			store := storagetest.New(t)
			group := &models.ClientGroup{Name: "Wake test"}
			broadcast := "127.0.0.1:invalid"
			if override {
				group.WOLBroadcastAddr = broadcast
				broadcast = "127.0.0.1"
			}
			if err := store.CreateClientGroup(group); err != nil {
				t.Fatal(err)
			}
			client := &models.Client{MACAddress: "02:00:00:00:00:01", Enabled: true, ClientGroupID: &group.ID}
			if err := store.CreateClient(client); err != nil {
				t.Fatal(err)
			}
			handler := &Handler{storage: store, wolBroadcastAddr: broadcast}
			request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/client-groups/wake?id=%d", group.ID), nil)
			recorder := httptest.NewRecorder()
			handler.WakeClientGroup(recorder, request)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
			}
			var response Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Success || !strings.Contains(response.Error, "failed to resolve broadcast address") {
				t.Fatalf("unexpected response: %+v", response)
			}
		})
	}
}

func TestWakeClientGroupNoEnabledMembers(t *testing.T) {
	for _, disabledMember := range []bool{false, true} {
		t.Run(fmt.Sprintf("disabled member=%t", disabledMember), func(t *testing.T) {
			store := storagetest.New(t)
			group := &models.ClientGroup{Name: "Empty wake"}
			if err := store.CreateClientGroup(group); err != nil {
				t.Fatal(err)
			}
			if disabledMember {
				client := &models.Client{MACAddress: "02:00:00:00:00:01", ClientGroupID: &group.ID}
				if err := store.CreateClient(client); err != nil {
					t.Fatal(err)
				}
				client.Enabled = false
				if err := store.UpdateClient(client.MACAddress, client); err != nil {
					t.Fatal(err)
				}
			}
			handler := &Handler{storage: store, wolBroadcastAddr: "127.0.0.1:invalid"}
			recorder := httptest.NewRecorder()
			handler.WakeClientGroup(recorder, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/client-groups/wake?id=%d", group.ID), nil))
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "No enabled members to wake") {
				t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStartGroupWakeFirstSendFailure(t *testing.T) {
	sendErr := errors.New("send failed")
	calls := make(chan string, 2)
	err := startGroupWake([]string{"first", "second"}, "broadcast", 0, func(mac, broadcast string) error {
		calls <- mac
		return sendErr
	})
	if !errors.Is(err, sendErr) || !strings.Contains(err.Error(), "first (broadcast: broadcast)") {
		t.Fatalf("unexpected error: %v", err)
	}
	if mac := <-calls; mac != "first" {
		t.Fatalf("first call = %q", mac)
	}
	select {
	case mac := <-calls:
		t.Fatalf("sent to %q after startup failure", mac)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestStartGroupWakeStaggerAndLaterFailure(t *testing.T) {
	for _, gap := range []time.Duration{0, 20 * time.Millisecond} {
		t.Run(gap.String(), func(t *testing.T) {
			type packet struct {
				mac       string
				broadcast string
				at        time.Time
			}
			packets := make(chan packet, 3)
			macs := collectEnabledMACs([]*models.Client{
				{MACAddress: "first", Enabled: true},
				{MACAddress: "disabled", Enabled: false},
				{MACAddress: "second", Enabled: true},
				{MACAddress: "third", Enabled: true},
			})
			err := startGroupWake(macs, "broadcast", gap, func(mac, broadcast string) error {
				packets <- packet{mac: mac, broadcast: broadcast, at: time.Now()}
				if mac == "second" {
					return errors.New("later failure")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(packets) == 0 {
				t.Fatal("returned before first send")
			}
			var previous time.Time
			for _, expected := range []string{"first", "second", "third"} {
				select {
				case got := <-packets:
					if got.mac != expected || got.broadcast != "broadcast" {
						t.Fatalf("unexpected packet: %+v", got)
					}
					if !previous.IsZero() && got.at.Sub(previous) < gap {
						t.Fatalf("stagger = %s, want at least %s", got.at.Sub(previous), gap)
					}
					previous = got.at
				case <-time.After(time.Second):
					t.Fatalf("missing send to %s", expected)
				}
			}
		})
	}
}

func TestStartGroupWakeEmpty(t *testing.T) {
	err := startGroupWake(nil, "broadcast", 0, func(string, string) error {
		t.Fatal("attempted send without members")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
