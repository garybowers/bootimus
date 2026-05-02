package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type PowerAction string

const (
	PowerOn               PowerAction = "On"
	PowerOff              PowerAction = "ForceOff"
	PowerRestart          PowerAction = "ForceRestart"
	PowerGracefulShutdown PowerAction = "GracefulShutdown"
	PowerGracefulRestart  PowerAction = "GracefulRestart"
)

type Client struct {
	Host     string
	Port     int
	Username string
	Password string
	Insecure bool
	http     *http.Client
}

func New(host string, port int, username, password string, insecure bool) *Client {
	if port == 0 {
		port = 443
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &Client{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Insecure: insecure,
		http:     &http.Client{Timeout: 15 * time.Second, Transport: transport},
	}
}

func (c *Client) PowerState(ctx context.Context) (string, error) {
	system, err := c.firstSystem(ctx)
	if err != nil {
		return "", err
	}
	return system.PowerState, nil
}

func (c *Client) SetPower(ctx context.Context, action PowerAction) error {
	system, err := c.firstSystem(ctx)
	if err != nil {
		return err
	}
	resetURL := system.resetActionTarget()
	if resetURL == "" {
		return fmt.Errorf("no ComputerSystem.Reset action advertised")
	}
	body, _ := json.Marshal(map[string]string{"ResetType": string(action)})
	req, err := c.newRequest(ctx, "POST", resetURL, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("BMC rejected %s: HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

type systemsCollection struct {
	Members []struct {
		ODataID string `json:"@odata.id"`
	} `json:"Members"`
}

type system struct {
	ODataID    string `json:"@odata.id"`
	PowerState string `json:"PowerState"`
	Actions    struct {
		Reset struct {
			Target string `json:"target"`
		} `json:"#ComputerSystem.Reset"`
	} `json:"Actions"`
}

func (s *system) resetActionTarget() string {
	return s.Actions.Reset.Target
}

func (c *Client) firstSystem(ctx context.Context) (*system, error) {
	var coll systemsCollection
	if err := c.getJSON(ctx, "/redfish/v1/Systems/", &coll); err != nil {
		return nil, err
	}
	if len(coll.Members) == 0 {
		return nil, fmt.Errorf("no systems returned by BMC")
	}
	var sys system
	if err := c.getJSON(ctx, coll.Members[0].ODataID, &sys); err != nil {
		return nil, err
	}
	return &sys, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := c.newRequest(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := fmt.Sprintf("https://%s:%d%s", c.Host, c.Port, path)
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
