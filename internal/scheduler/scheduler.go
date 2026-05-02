package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage"

	"github.com/robfig/cron/v3"
)

type Runner func(ctx context.Context, task *models.ScheduledTask) (status string, errMsg string)

type Scheduler struct {
	store   storage.Storage
	runner  Runner
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[uint]cron.EntryID
}

func New(store storage.Storage, runner Runner) *Scheduler {
	return &Scheduler{
		store:   store,
		runner:  runner,
		cron:    cron.New(),
		entries: make(map[uint]cron.EntryID),
	}
}

func (s *Scheduler) Start() {
	s.cron.Start()
	if err := s.Reload(); err != nil {
		log.Printf("scheduler: initial load failed: %v", err)
	}
}

func (s *Scheduler) Stop() {
	if s.cron != nil {
		ctx := s.cron.Stop()
		<-ctx.Done()
	}
}

func (s *Scheduler) Reload() error {
	if s.store == nil {
		return nil
	}
	tasks, err := s.store.ListScheduledTasks()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	live := make(map[uint]bool, len(tasks))
	for _, t := range tasks {
		if t.Enabled {
			live[t.ID] = true
		}
	}
	for id, entryID := range s.entries {
		if !live[id] {
			s.cron.Remove(entryID)
			delete(s.entries, id)
		}
	}

	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		if existing, ok := s.entries[t.ID]; ok {
			s.cron.Remove(existing)
			delete(s.entries, t.ID)
		}
		task := *t
		entryID, err := s.cron.AddFunc(t.CronExpr, func() {
			s.runTask(task)
		})
		if err != nil {
			log.Printf("scheduler: invalid cron %q for task %d (%s): %v", t.CronExpr, t.ID, t.Name, err)
			continue
		}
		s.entries[t.ID] = entryID
	}
	log.Printf("scheduler: %d active task(s) loaded", len(s.entries))
	return nil
}

func (s *Scheduler) RunNow(id uint) error {
	t, err := s.store.GetScheduledTask(id)
	if err != nil {
		return err
	}
	go s.runTask(*t)
	return nil
}

func (s *Scheduler) runTask(t models.ScheduledTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	status, errMsg := s.runner(ctx, &t)
	if err := s.store.RecordScheduledTaskRun(t.ID, status, errMsg); err != nil {
		log.Printf("scheduler: failed to record run for task %d: %v", t.ID, err)
	}
	log.Printf("scheduler: task %d (%s) → %s%s", t.ID, t.Name, status, func() string {
		if errMsg != "" {
			return ": " + errMsg
		}
		return ""
	}())
}
