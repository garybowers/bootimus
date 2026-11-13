package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"bootimus/internal/auth"
	"bootimus/internal/models"
	"bootimus/internal/storage"
)

const (
	timelineKindBoot    = "boot"
	timelineKindInstall = "install"
	timelineKindDisk    = "disk"
	timelineKindDHCP    = "dhcp"
	timelineKindAudit   = "audit"

	timelineStatusOK      = "ok"
	timelineStatusFailed  = "failed"
	timelineStatusPending = "pending"
	timelineStatusInfo    = "info"

	timelineSourceLimit = 500
)

type timelineItem struct {
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status"`
	SourceID  string    `json:"source_id,omitempty"`
	cursorKey string
}

type timelineCursor struct {
	Time time.Time `json:"time"`
	Key  string    `json:"key,omitempty"`
}

func parseTimelineCursor(raw string) (timelineCursor, error) {
	if raw == "" {
		return timelineCursor{}, nil
	}
	if before, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return timelineCursor{Time: before}, nil
	}
	invalid := errors.New("before must be a timeline cursor or RFC 3339 timestamp")
	encoded, ok := strings.CutPrefix(raw, "v1.")
	if !ok || len(encoded) > 512 {
		return timelineCursor{}, invalid
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return timelineCursor{}, invalid
	}
	var cursor timelineCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Time.IsZero() || cursor.Key == "" {
		return timelineCursor{}, invalid
	}
	return cursor, nil
}

type timelinePage struct {
	Items      []timelineItem `json:"items"`
	NextBefore string         `json:"next_before,omitempty"`
	HasMore    bool           `json:"has_more"`
}

func (s *Server) clientTimeline(mac string, before timelineCursor, limit int, includeAudit bool) (timelinePage, error) {
	var items []timelineItem
	store := s.config.Storage

	if logs, err := store.GetBootLogsByMAC(mac, timelineSourceLimit); err != nil {
		return timelinePage{}, err
	} else {
		for _, l := range logs {
			items = append(items, bootLogItem(l))
		}
	}
	if recs, err := store.ListInstallRecords(mac, timelineSourceLimit); err != nil {
		return timelinePage{}, err
	} else {
		for _, r := range recs {
			items = append(items, installItems(r)...)
		}
	}
	if jobs, err := store.ListDiskJobs(mac, timelineSourceLimit); err != nil {
		return timelinePage{}, err
	} else {
		for _, j := range jobs {
			items = append(items, diskJobItem(j))
		}
	}
	for _, ev := range s.proxyDHCPEvents() {
		if !strings.EqualFold(ev.MAC, mac) {
			continue
		}
		status := timelineStatusInfo
		switch ev.Outcome {
		case "replied":
			status = timelineStatusOK
		case "failed":
			status = timelineStatusFailed
		}
		items = append(items, timelineItem{
			Time:      ev.Time,
			Kind:      timelineKindDHCP,
			Title:     "proxyDHCP " + ev.MessageType + " from " + ev.Source,
			Detail:    ev.Decision,
			Status:    status,
			cursorKey: "dhcp:" + strconv.FormatUint(ev.Sequence, 10),
		})
	}
	if includeAudit {
		entries, _, err := store.ListAuditLogs(storage.AuditLogFilter{Text: mac, Limit: timelineSourceLimit})
		if err != nil {
			return timelinePage{}, err
		}
		for _, e := range entries {
			status := timelineStatusOK
			if e.Status >= 400 {
				status = timelineStatusFailed
			}
			items = append(items, timelineItem{
				Time:      e.CreatedAt,
				Kind:      timelineKindAudit,
				Title:     e.Summary,
				Detail:    e.Username + " · " + e.Method + " " + e.Path + " → " + strconv.Itoa(e.Status),
				Status:    status,
				SourceID:  "audit:" + strconv.FormatUint(uint64(e.ID), 10),
				cursorKey: "audit:" + strconv.FormatUint(uint64(e.ID), 10),
			})
		}
	}

	sort.Slice(items, func(left, right int) bool {
		if items[left].Time.Equal(items[right].Time) {
			return items[left].cursorKey < items[right].cursorKey
		}
		return items[left].Time.After(items[right].Time)
	})

	filtered := items[:0]
	for _, it := range items {
		if before.Time.IsZero() || it.Time.Before(before.Time) ||
			(it.Time.Equal(before.Time) && before.Key != "" && it.cursorKey > before.Key) {
			filtered = append(filtered, it)
		}
	}
	page := timelinePage{Items: []timelineItem{}}
	if len(filtered) > limit {
		page.Items = filtered[:limit]
		page.HasMore = true
		last := page.Items[limit-1]
		cursor, err := json.Marshal(timelineCursor{Time: last.Time.UTC(), Key: last.cursorKey})
		if err != nil {
			return timelinePage{}, err
		}
		page.NextBefore = "v1." + base64.RawURLEncoding.EncodeToString(cursor)
	} else {
		page.Items = filtered
	}
	return page, nil
}

func bootLogItem(l models.BootLog) timelineItem {
	status := timelineStatusOK
	if !l.Success {
		status = timelineStatusFailed
	}
	detail := l.IPAddress
	if l.ErrorMsg != "" {
		detail = strings.TrimSpace(detail + " " + l.ErrorMsg)
	}
	title := "Booted " + l.ImageName
	if l.ImageName == "" {
		title = "Boot attempt"
	}
	if l.Source == models.BootSourceNativeTFTP {
		title = l.ImageName
		detail = strings.TrimSpace(detail + " (file transferred; OS startup not confirmed)")
	}
	sourceID := "boot:" + strconv.FormatUint(uint64(l.ID), 10)
	return timelineItem{Time: l.CreatedAt, Kind: timelineKindBoot, Title: title, Detail: detail, Status: status, SourceID: sourceID, cursorKey: sourceID}
}

func installItems(r models.InstallRecord) []timelineItem {
	id := "install:" + strconv.FormatUint(uint64(r.ID), 10)
	out := []timelineItem{{Time: r.BootedAt, Kind: timelineKindInstall, Title: "Install boot of " + r.ImageName, Status: timelineStatusInfo, SourceID: id, cursorKey: id + ":boot"}}
	if r.InstallingAt != nil {
		out = append(out, timelineItem{Time: *r.InstallingAt, Kind: timelineKindInstall, Title: "Install started: " + r.ImageName, Detail: "auto-install file fetched", Status: timelineStatusPending, SourceID: id, cursorKey: id + ":started"})
	}
	if r.FinishedAt != nil {
		status := timelineStatusOK
		title := "Install completed: " + r.ImageName
		if r.State == models.InstallStateFailed {
			status = timelineStatusFailed
			title = "Install failed: " + r.ImageName
		}
		out = append(out, timelineItem{Time: *r.FinishedAt, Kind: timelineKindInstall, Title: title, Detail: r.Message, Status: status, SourceID: id, cursorKey: id + ":finished"})
	}
	return out
}

func diskJobItem(j *models.DiskJob) timelineItem {
	name := ""
	if j.DiskImage != nil {
		name = j.DiskImage.Name
	}
	title := strings.ToUpper(j.Kind[:1]) + j.Kind[1:] + " " + name
	status := timelineStatusPending
	switch j.State {
	case models.DiskJobStateCompleted:
		status = timelineStatusOK
	case models.DiskJobStateFailed:
		status = timelineStatusFailed
	case models.DiskJobStateCancelled:
		status = timelineStatusInfo
	}
	when := j.CreatedAt
	if j.FinishedAt != nil {
		when = *j.FinishedAt
	} else if j.StartedAt != nil {
		when = *j.StartedAt
	}
	detail := "disk " + j.Disk + " · " + j.State
	if j.Message != "" {
		detail += " · " + j.Message
	}
	sourceID := "disk:" + strconv.FormatUint(uint64(j.ID), 10)
	return timelineItem{Time: when, Kind: timelineKindDisk, Title: title, Detail: detail, Status: status, SourceID: sourceID, cursorKey: sourceID}
}

func (s *Server) handleClientTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}
	if s.config.Storage == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Timeline requires a database"})
		return
	}
	mac := normaliseMenuMAC(r.URL.Query().Get("mac"))
	if mac == "unknown" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "mac is required"})
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	before, err := parseTimelineCursor(r.URL.Query().Get("before"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	includeAudit := s.config.Auth == nil || auth.IsAdminFromContext(r.Context())
	page, err := s.clientTimeline(mac, before, limit, includeAudit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": page})
}
