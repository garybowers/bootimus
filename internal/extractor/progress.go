package extractor

import "sync"

type ProgressReporter struct {
	mu             sync.RWMutex
	stage          string
	totalBytes     int64
	extractedBytes int64
}

type ProgressSnapshot struct {
	Stage   string  `json:"stage"`
	Percent float64 `json:"percent"`
}

func NewProgressReporter() *ProgressReporter {
	return &ProgressReporter{stage: "Starting..."}
}

func (p *ProgressReporter) SetStage(stage string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.stage = stage
	p.mu.Unlock()
}

func (p *ProgressReporter) SetTotalBytes(n int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.totalBytes = n
	p.mu.Unlock()
}

func (p *ProgressReporter) AddBytes(n int64) {
	if p == nil || n <= 0 {
		return
	}
	p.mu.Lock()
	p.extractedBytes += n
	p.mu.Unlock()
}

func (p *ProgressReporter) Snapshot() ProgressSnapshot {
	if p == nil {
		return ProgressSnapshot{}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var percent float64
	if p.totalBytes > 0 {
		percent = float64(p.extractedBytes) / float64(p.totalBytes) * 100
		if percent > 100 {
			percent = 100
		}
	}
	return ProgressSnapshot{Stage: p.stage, Percent: percent}
}
