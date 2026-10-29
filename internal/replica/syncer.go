package replica

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"bootimus/internal/storage"
)

type Deps struct {
	Store               storage.Storage
	HTTPClient          *http.Client
	DataDir             string
	ISODir              string
	AutoInstall         *autoinstall.Library
	Extract             func(filename string) error
	SelectBootloaderSet func(name string) error
	BootloaderSetExists func(name string) bool
	ToolDir             func(name string) string
}

type Syncer struct {
	cfg     Config
	deps    Deps
	primary *primary

	mu           sync.RWMutex
	status       Status
	bootBase     string
	manifestSeen map[string]string

	runMu    sync.Mutex
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

type manifest struct {
	BootBaseURL   string                  `json:"boot_base_url"`
	BootloaderSet string                  `json:"bootloader_set"`
	Version       string                  `json:"version"`
	Kinds         map[string]manifestKind `json:"kinds"`
}

type manifestKind struct {
	Count     int    `json:"count"`
	UpdatedAt string `json:"updated_at"`
}

func New(cfg Config, deps Deps) *Syncer {
	if cfg.Interval == 0 {
		cfg.Interval = DefaultInterval
	}
	s := &Syncer{
		cfg:          cfg,
		deps:         deps,
		primary:      newPrimary(cfg),
		manifestSeen: map[string]string{},
		stop:         make(chan struct{}),
	}
	if deps.HTTPClient != nil {
		s.primary.http = deps.HTTPClient
	}
	s.status = Status{PrimaryURL: cfg.DisplayURL(), SyncISOs: cfg.SyncISOs, IntervalSeconds: int(cfg.Interval / time.Second)}
	if deps.Store != nil {
		if state, err := deps.Store.GetReplicaState(); err == nil {
			if saved, ok := parseReport(state.Report); ok {
				saved.PrimaryURL = s.status.PrimaryURL
				saved.SyncISOs = s.status.SyncISOs
				saved.IntervalSeconds = s.status.IntervalSeconds
				saved.Running = false
				s.status = saved
			}
		}
	}
	return s
}

func (s *Syncer) PrimaryURL() string {
	return s.cfg.DisplayURL()
}

func (s *Syncer) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.RunNow(context.Background())
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.RunNow(context.Background())
			}
		}
	}()
}

func (s *Syncer) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

func (s *Syncer) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.status
	out.Kinds = append([]KindResult(nil), s.status.Kinds...)
	return out
}

func (s *Syncer) BootBaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bootBase
}

func (s *Syncer) ImageIsLocal(filename string) bool {
	if s.deps.ISODir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(s.deps.ISODir, filepath.FromSlash(filename)))
	return err == nil && !info.IsDir()
}

type kindFunc func(ctx context.Context, run *syncRun) (KindResult, error)

type syncRun struct {
	manifest          *manifest
	imageGroups       map[uint]uint
	sanTargets        map[uint]uint
	clientGroups      map[uint]uint
	imageIDs          map[uint]string
	imagePolicyFailed bool
}

func (s *Syncer) RunNow(ctx context.Context) Status {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	s.mu.Lock()
	s.status.Running = true
	s.mu.Unlock()

	started := time.Now()
	run := &syncRun{imageGroups: map[uint]uint{}, sanTargets: map[uint]uint{}, clientGroups: map[uint]uint{}, imageIDs: map[uint]string{}}
	var results []KindResult
	var firstErr error

	mf, err := s.fetchManifest(ctx)
	if err != nil {
		firstErr = err
		log.Printf("Replica: manifest unavailable, syncing every kind: %v", err)
	} else {
		run.manifest = mf
		s.mu.Lock()
		if mf.BootBaseURL != "" {
			s.bootBase = mf.BootBaseURL
		}
		s.mu.Unlock()
	}

	unreachable := false
	for _, step := range s.steps() {
		if unreachable {
			results = append(results, KindResult{Name: step.name, Skipped: true, Note: "primary unreachable"})
			continue
		}
		if sig, ok := s.manifestSignature(mf, step.name); ok && s.manifestSeen[step.name] == sig {
			results = append(results, KindResult{Name: step.name, Skipped: true, Note: "unchanged on the primary"})
			continue
		}
		stepStart := time.Now()
		res, err := step.fn(ctx, run)
		res.Name = step.name
		res.DurationMs = time.Since(stepStart).Milliseconds()
		if err != nil {
			if step.name == "images" {
				run.imagePolicyFailed = true
			}
			res.Error = err.Error()
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", step.name, err)
			}
			log.Printf("Replica: %s sync failed: %v", step.name, err)
			if isUnreachable(err) {
				unreachable = true
			}
		} else if sig, ok := s.manifestSignature(mf, step.name); ok {
			s.manifestSeen[step.name] = sig
		}
		results = append(results, res)
	}

	now := time.Now()
	s.mu.Lock()
	s.status.Running = false
	s.status.LastRun = &now
	s.status.Kinds = results
	if firstErr != nil {
		s.status.LastError = firstErr.Error()
	} else {
		s.status.LastError = ""
		s.status.LastSuccess = &now
	}
	status := s.status
	s.mu.Unlock()

	if s.deps.Store != nil {
		state := &models.ReplicaState{LastSync: status.LastSuccess, Report: status.report()}
		if err := s.deps.Store.SaveReplicaState(state); err != nil {
			log.Printf("Replica: failed to persist sync state: %v", err)
		}
	}
	log.Printf("Replica: sync finished in %s (%d kinds, error: %v)", time.Since(started).Round(time.Millisecond), len(results), firstErr)
	return s.Status()
}

type step struct {
	name string
	fn   kindFunc
}

func (s *Syncer) steps() []step {
	return []step{
		{"san_targets", s.syncSANTargets},
		{"image_groups", s.syncImageGroups},
		{"images", s.syncImages},
		{"client_groups", s.syncClientGroups},
		{"clients", s.syncClients},
		{"theme", s.syncTheme},
		{"profiles", s.syncProfiles},
		{"tools", s.syncTools},
		{"autoinstall_files", s.syncAutoInstallFiles},
		{"custom_files", s.syncCustomFiles},
		{"bootloader_set", s.syncBootloaderSet},
	}
}

var noSkipKinds = map[string]bool{
	"images":            true,
	"clients":           true,
	"autoinstall_files": true,
	"tools":             true,
	"bootloader_set":    true,
	"theme":             true,
}

func (s *Syncer) manifestSignature(mf *manifest, kind string) (string, bool) {
	if mf == nil || noSkipKinds[kind] {
		return "", false
	}
	k, ok := mf.Kinds[kind]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d@%s", k.Count, k.UpdatedAt), true
}

func (s *Syncer) fetchManifest(ctx context.Context) (*manifest, error) {
	var mf manifest
	if err := s.primary.getJSON(ctx, "/api/sync/manifest", &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func isUnreachable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "primary unreachable")
}
