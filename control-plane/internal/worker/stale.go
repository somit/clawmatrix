package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"control-plane/internal/api"
	"control-plane/internal/database"
)

const (
	heartbeatInterval = 30 * time.Second
	staleAfterMissed  = 3
	checkInterval     = 30 * time.Second
)

const metricRetention = 7 * 24 * time.Hour // 7 days

var hub *api.Hub

func StaleLoop(h *api.Hub) {
	hub = h
	ticker := time.NewTicker(checkInterval)
	purgeTicker := time.NewTicker(1 * time.Hour)
	for {
		select {
		case <-ticker.C:
			checkStale()
		case <-purgeTicker.C:
			cutoff := time.Now().UTC().Add(-metricRetention)
			if n, _ := database.PurgeMetrics(cutoff); n > 0 {
				logWorker("METRICS_PURGED", "", fmt.Sprintf("%d rows older than 7d", n))
			}
			logCutoff := time.Now().UTC().Add(-24 * time.Hour)
			if n, _ := database.PurgeRequestLogs(logCutoff); n > 0 {
				logWorker("LOGS_PURGED", "", fmt.Sprintf("%d rows older than 24h", n))
			}
			auditCutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
			if n, _ := database.PurgeAuditEvents(auditCutoff); n > 0 {
				logWorker("AUDIT_PURGED", "", fmt.Sprintf("%d rows older than 7d", n))
			}
			cronCutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
			if n, _ := database.PurgeCronExecutions(cronCutoff); n > 0 {
				logWorker("CRON_EXECUTIONS_PURGED", "", fmt.Sprintf("%d rows older than 7d", n))
			}
		}
	}
}

func checkStale() {
	agents, err := database.GetAgentsForStaleCheck()
	if err != nil {
		return
	}

	now := time.Now().UTC()
	for _, a := range agents {
		// TTL check
		regName := database.GetAgentRegistrationName(&a)
		if regName == "" {
			continue
		}
		reg, err := database.GetRegistrationByName(regName)
		if err != nil {
			continue
		}

		if reg.TTLMinutes > 0 {
			age := now.Sub(a.RegisteredAt)
			if age > time.Duration(reg.TTLMinutes)*time.Minute {
				database.UpdateAgentStatus(a.ID, "kill", "ttl_exceeded")
				logWorker("TTL_KILL", a.ID, fmt.Sprintf("ran %s", age.Truncate(time.Second)))
				hub.Broadcast(api.Event{Type: "agent:killed", Data: J{"id": a.ID, "reason": "ttl_exceeded"}})
				continue
			}
		}

		// Stale check
		missed := now.Sub(a.LastHeartbeat)
		threshold := heartbeatInterval * time.Duration(staleAfterMissed)
		if missed > threshold && a.Status == "healthy" {
			database.UpdateAgentStatus(a.ID, "stale", "")
			logWorker("STALE", a.ID, fmt.Sprintf("missed %s", missed.Truncate(time.Second)))
			hub.Broadcast(api.Event{Type: "agent:stale", Data: J{"id": a.ID}})
		}
	}
}

type J = map[string]any

func logWorker(action, id, detail string) {
	entry := J{"ts": time.Now().UTC().UTC().Format(time.RFC3339), "action": action, "id": id}
	if detail != "" {
		entry["detail"] = detail
	}
	b, _ := json.Marshal(entry)
	log.Println(string(b))
}
