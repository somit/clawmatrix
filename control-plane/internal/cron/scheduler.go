package cron

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"control-plane/internal/api"
	"control-plane/internal/database"
)

type Scheduler struct {
	c       *cron.Cron
	hub     *api.Hub
	entries map[uint]cron.EntryID  // cronJobID → robfig entryID (recurring)
	timers  map[uint]*time.Timer   // cronJobID → timer (one-time RunAt)
	mu      sync.Mutex
	client  *http.Client
}

func NewScheduler(hub *api.Hub) *Scheduler {
	return &Scheduler{
		c:       cron.New(),
		hub:     hub,
		entries: make(map[uint]cron.EntryID),
		timers:  make(map[uint]*time.Timer),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (s *Scheduler) Start() {
	jobs, err := database.ListEnabledCronJobs()
	if err != nil {
		log.Printf("[cron] failed to load jobs: %v", err)
	}
	for i := range jobs {
		if err := s.addJobLocked(&jobs[i]); err != nil {
			log.Printf("[cron] failed to schedule %q (id=%d): %v", jobs[i].Name, jobs[i].ID, err)
		}
	}
	s.c.Start()
	log.Printf("[cron] started with %d jobs", len(jobs))
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}

func (s *Scheduler) AddJob(job *database.CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addJobLocked(job)
}

func (s *Scheduler) RemoveJob(cronJobID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[cronJobID]; ok {
		s.c.Remove(entryID)
		delete(s.entries, cronJobID)
	}
	if timer, ok := s.timers[cronJobID]; ok {
		timer.Stop()
		delete(s.timers, cronJobID)
	}
}

func (s *Scheduler) UpdateJob(job *database.CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[job.ID]; ok {
		s.c.Remove(entryID)
		delete(s.entries, job.ID)
	}
	if !job.Enabled {
		return nil
	}
	return s.addJobLocked(job)
}

// TriggerJob runs a cron job immediately, independent of its schedule.
func (s *Scheduler) TriggerJob(job *database.CronJob) {
	go s.executeJob(job.ID)
}

// NextRunTime returns the next scheduled run for a job, if scheduled.
func (s *Scheduler) NextRunTime(cronJobID uint) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if it's a RunAt timer job
	if _, ok := s.timers[cronJobID]; ok {
		// RunAt time is stored in DB; return nil here so cronToJSON falls back to DB value
		return nil
	}

	entryID, ok := s.entries[cronJobID]
	if !ok {
		return nil
	}
	entry := s.c.Entry(entryID)
	if entry.Next.IsZero() {
		return nil
	}
	t := entry.Next
	return &t
}

func (s *Scheduler) addJobLocked(job *database.CronJob) error {
	jobID := job.ID

	// One-time RunAt job
	if job.RunAt != nil && !job.RunAt.IsZero() {
		delay := time.Until(*job.RunAt)
		if delay < 0 {
			// Already past — fire immediately
			delay = 0
		}
		timer := time.AfterFunc(delay, func() {
			s.executeJob(jobID)
			// Auto-disable after execution
			database.UpdateCronJob(jobID, map[string]any{"enabled": false, "next_run_at": nil})
			s.mu.Lock()
			delete(s.timers, jobID)
			s.mu.Unlock()
		})
		s.timers[jobID] = timer
		database.UpdateCronJob(jobID, map[string]any{"next_run_at": *job.RunAt})
		return nil
	}

	// Recurring cron job
	schedule := job.Schedule
	if job.Timezone != "" && job.Timezone != "UTC" {
		schedule = fmt.Sprintf("CRON_TZ=%s %s", job.Timezone, job.Schedule)
	}

	entryID, err := s.c.AddFunc(schedule, func() {
		s.executeJob(jobID)
	})
	if err != nil {
		return fmt.Errorf("invalid schedule %q: %w", job.Schedule, err)
	}
	s.entries[jobID] = entryID

	// Update next run time in DB
	next := s.c.Entry(entryID).Next
	if !next.IsZero() {
		database.UpdateCronJob(jobID, map[string]any{"next_run_at": next})
	}

	return nil
}

func (s *Scheduler) executeJob(cronJobID uint) {
	job, err := database.GetCronJob(cronJobID)
	if err != nil {
		log.Printf("[cron] job %d not found: %v", cronJobID, err)
		return
	}

	start := time.Now().UTC()

	// Find a healthy agent with a chatUrl
	agent := s.findChatAgent(job)
	if agent == nil {
		s.recordExecution(job, "", "no_agent", "no healthy agent with chatUrl", 0)
		return
	}
	chatURL := extractChatURL(agent)
	agentID := agent.ID

	// POST message to agent
	payload := map[string]string{"message": job.Message}
	session := job.Session
	if session == "" {
		session = fmt.Sprintf("cron:%d:%s", job.ID, job.Name)
	}
	payload["session"] = session

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", chatURL, bytes.NewReader(body))
	if err != nil {
		s.recordExecution(job, agentID, "error", err.Error(), time.Since(start).Milliseconds())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.recordExecution(job, agentID, "error", err.Error(), time.Since(start).Milliseconds())
		return
	}
	resp.Body.Close()
	durationMs := time.Since(start).Milliseconds()

	if resp.StatusCode >= 400 {
		s.recordExecution(job, agentID, "error", fmt.Sprintf("HTTP %d", resp.StatusCode), durationMs)
		return
	}

	s.recordExecution(job, agentID, "ok", "", durationMs)
}

func (s *Scheduler) findChatAgent(job *database.CronJob) *database.Agent {
	if job.AgentProfileName == "" {
		return nil
	}

	candidates, err := database.GetAgentsByName(job.AgentProfileName)
	if err != nil {
		return nil
	}

	healthy := func(a *database.Agent) bool {
		return a.Status == "healthy" && extractChatURL(a) != ""
	}

	// Prefer agent from the target registration
	if job.RegistrationName != "" {
		for i := range candidates {
			if database.GetAgentRegistrationName(&candidates[i]) == job.RegistrationName && healthy(&candidates[i]) {
				return &candidates[i]
			}
		}
	}

	// Fall back to any healthy agent with the same profile name
	for i := range candidates {
		if healthy(&candidates[i]) {
			return &candidates[i]
		}
	}

	return nil
}

func extractChatURL(agent *database.Agent) string {
	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	url, _ := meta["chatUrl"].(string)
	return url
}

func (s *Scheduler) recordExecution(job *database.CronJob, agentID, status, errMsg string, durationMs int64) {
	now := time.Now().UTC()

	// Record execution
	exec := &database.CronExecution{
		CronJobID:  job.ID,
		AgentID:    agentID,
		Status:     status,
		Error:      errMsg,
		DurationMs: durationMs,
		CreatedAt:  now,
	}
	database.InsertCronExecution(exec)

	// Update job state
	updates := map[string]any{
		"last_run_at": now,
		"last_status": status,
		"last_error":  errMsg,
	}

	// Update next run from robfig entry
	s.mu.Lock()
	if entryID, ok := s.entries[job.ID]; ok {
		next := s.c.Entry(entryID).Next
		if !next.IsZero() {
			updates["next_run_at"] = next
		}
	}
	s.mu.Unlock()

	database.UpdateCronJob(job.ID, updates)

	// Broadcast SSE
	eventType := "cron:executed"
	if status != "ok" {
		eventType = "cron:failed"
	}
	data := map[string]any{
		"id": job.ID, "name": job.Name, "agentId": agentID,
		"status": status, "durationMs": durationMs,
	}
	if errMsg != "" {
		data["error"] = errMsg
	}
	s.hub.Broadcast(api.Event{Type: eventType, Data: data})

	log.Printf("[cron] %s job=%d name=%q agent=%s status=%s duration=%dms",
		eventType, job.ID, job.Name, agentID, status, durationMs)
}
