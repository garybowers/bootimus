package replica

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrForbidden = errors.New("the primary refused the request; the token needs a higher scope")

type primary struct {
	base  string
	token string
	http  *http.Client
}

func newPrimary(cfg Config) *primary {
	return &primary{
		base:  strings.TrimRight(strings.TrimSpace(cfg.PrimaryURL), "/"),
		token: strings.TrimSpace(cfg.Token),
		http:  &http.Client{Timeout: 0},
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func (p *primary) request(ctx context.Context, path string) (*http.Response, error) {
	return p.requestWithClient(ctx, path, p.http)
}

func (p *primary) requestWithClient(ctx context.Context, path string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("User-Agent", "bootimus-replica/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("primary unreachable: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, ErrForbidden
	}
	return resp, nil
}

func (p *primary) getJSON(ctx context.Context, path string, out interface{}) error {
	return p.getJSONWithClient(ctx, path, out, p.http, 256<<20)
}

func (p *primary) getJSONWithClient(ctx context.Context, path string, out interface{}, client *http.Client, limit int64) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	resp, err := p.requestWithClient(ctx, path, client)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("primary response for %s exceeds the size limit", path)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("primary has no %s endpoint", path)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("primary returned HTTP %d with a non-JSON body for %s", resp.StatusCode, path)
	}
	if resp.StatusCode >= 400 || !env.Success {
		msg := env.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("primary rejected %s: %s", path, msg)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

func (p *primary) getBootVerifiers(ctx context.Context, out interface{}) error {
	parsed, err := url.Parse(p.base)
	if err != nil || parsed.Scheme != "https" {
		return fmt.Errorf("boot verifier sync requires an HTTPS primary URL")
	}
	client := *p.http
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return p.getJSONWithClient(ctx, "/api/sync/boot-verifiers", out, &client, 4<<20)
}

func (p *primary) download(ctx context.Context, path, dest string) (int64, error) {
	resp, err := p.request(ctx, path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("primary returned HTTP %d for %s", resp.StatusCode, path)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".replica-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	n, copyErr := io.Copy(tmp, resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if copyErr != nil {
			return 0, copyErr
		}
		return 0, closeErr
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("short download of %s: got %d of %d bytes", path, n, resp.ContentLength)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}
	return n, nil
}

func assetPath(kind string, params map[string]string) string {
	q := url.Values{"kind": {kind}}
	for k, v := range params {
		q.Set(k, v)
	}
	return "/api/sync/asset?" + q.Encode()
}
