package database

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

// --- Registrations ---

func CreateRegistration(name, description, token string, allowlist []string, labels map[string]string, ttlMinutes int) (*Registration, error) {
	al, _ := json.Marshal(allowlist)
	lb, _ := json.Marshal(labels)
	if labels == nil {
		lb = []byte("{}")
	}
	t := &Registration{
		Name:        name,
		Description: description,
		TokenHash:   HashToken(token),
		EgressAllowlist: string(al),
		Labels:      string(lb),
		TTLMinutes:  ttlMinutes,
	}
	return t, DB.Create(t).Error
}

func GetRegistrationByName(name string) (*Registration, error) {
	var t Registration
	return &t, DB.Where("name = ?", name).First(&t).Error
}

func GetRegistrationByToken(token string) (*Registration, error) {
	var t Registration
	return &t, DB.Where("token_hash = ?", HashToken(token)).First(&t).Error
}


func ListRegistrations() ([]Registration, error) {
	var regs []Registration
	return regs, DB.Find(&regs).Error
}

func UpdateRegistration(name string, updates map[string]any) error {
	updates["updated_at"] = time.Now().UTC()
	result := DB.Model(&Registration{}).Where("name = ?", name).Updates(updates)
	if result.RowsAffected == 0 {
		return result.Error
	}
	// If name changed, cascade to agent profiles
	if newName, ok := updates["name"]; ok && newName != name {
		DB.Model(&AgentProfile{}).Where("registration = ?", name).Update("registration", newName)
	}
	return result.Error
}

func TouchMonitorHeartbeat(name string) error {
	now := time.Now().UTC()
	return DB.Model(&Registration{}).Where("name = ?", name).
		Update("monitor_last_seen", now).Error
}

func DeleteRegistration(name string) error {
	return DB.Where("name = ?", name).Delete(&Registration{}).Error
}

func IncrementRegistrations(name string) error {
	return DB.Model(&Registration{}).Where("name = ?", name).
		Update("total_registered", gorm.Expr("total_registered + 1")).Error
}

func SetArchived(name string, archived bool) error {
	return DB.Model(&Registration{}).Where("name = ?", name).Updates(map[string]any{
		"archived":   archived,
		"updated_at": time.Now().UTC(),
	}).Error
}

func GetAllowlist(t *Registration) []string {
	var list []string
	json.Unmarshal([]byte(t.EgressAllowlist), &list)
	return list
}

func GetLabels(t *Registration) map[string]string {
	var m map[string]string
	json.Unmarshal([]byte(t.Labels), &m)
	if m == nil {
		m = map[string]string{}
	}
	return m
}

func ListRegistrationsByLabel(key, value string) ([]Registration, error) {
	var regs []Registration
	return regs, DB.Where("json_extract(labels, ?) = ?", "$."+key, value).Find(&regs).Error
}

// --- Agent Profiles ---

func CreateAgentProfile(name, description, image, deploymentConfig, source string, registration *string, maxCount, ttlMinutes int) (*AgentProfile, error) {
	if deploymentConfig == "" {
		deploymentConfig = "{}"
	}
	t := &AgentProfile{
		Name:             name,
		Description:      description,
		Registration:     registration,
		Image:            image,
		MaxCount:         maxCount,
		TTLMinutes:       ttlMinutes,
		DeploymentConfig: deploymentConfig,
		Source:           source,
	}
	return t, DB.Create(t).Error
}

// UpsertAgentProfile creates the profile if it doesn't exist, or updates source if it does.
func UpsertAgentProfile(name, registrationName, source string) (*AgentProfile, error) {
	existing, err := GetAgentProfile(name)
	if err == nil {
		DB.Model(existing).Updates(map[string]any{
			"source":     source,
			"updated_at": time.Now().UTC(),
		})
		return existing, nil
	}
	reg := registrationName
	return CreateAgentProfile(name, "", "", "", source, &reg, 0, -1)
}

func GetAgentProfile(name string) (*AgentProfile, error) {
	var t AgentProfile
	return &t, DB.Where("name = ?", name).First(&t).Error
}

func ListAgentProfiles() ([]AgentProfile, error) {
	var profiles []AgentProfile
	return profiles, DB.Order("name").Find(&profiles).Error
}

func UpdateAgentProfile(name string, updates map[string]any) error {
	updates["updated_at"] = time.Now().UTC()
	return DB.Model(&AgentProfile{}).Where("name = ?", name).Updates(updates).Error
}

func DeleteAgentProfile(name string) error {
	return DB.Where("name = ?", name).Delete(&AgentProfile{}).Error
}

func CountAgentsByProfile(profileName string) (int64, error) {
	var count int64
	return count, DB.Model(&Agent{}).Where("agent_profile = ?", profileName).Count(&count).Error
}

// --- Connections ---

func CreateConnection(source, target string) (*Connection, error) {
	c := &Connection{Source: source, Target: target}
	return c, DB.Create(c).Error
}

func DeleteConnection(source, target string) error {
	result := DB.Where("source = ? AND target = ?", source, target).Delete(&Connection{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func ListConnectionsFrom(source string) ([]Connection, error) {
	var conns []Connection
	return conns, DB.Where("source = ?", source).Find(&conns).Error
}

func ListConnectionsTo(target string) ([]Connection, error) {
	var conns []Connection
	return conns, DB.Where("target = ?", target).Find(&conns).Error
}

func ListAllConnections() ([]Connection, error) {
	var conns []Connection
	return conns, DB.Order("source, target").Find(&conns).Error
}

func HasConnection(source, target string) bool {
	var count int64
	DB.Model(&Connection{}).Where("source = ? AND target = ?", source, target).Count(&count)
	return count > 0
}

// --- Agents ---

func CreateAgent(agent *Agent) error {
	return DB.Create(agent).Error
}

func GetAgent(id string) (*Agent, error) {
	var a Agent
	return &a, DB.Where("id = ?", id).First(&a).Error
}

func GetAgentsByName(name string) ([]Agent, error) {
	var agents []Agent
	return agents, DB.Where("agent_profile = ?", name).Find(&agents).Error
}

func GetAgentByToken(token string) (*Agent, error) {
	var a Agent
	return &a, DB.Where("token = ?", token).First(&a).Error
}

type HeartbeatStats struct {
	Allowed  int64
	Blocked  int64
	AvgMs    int64
	MinMs    int64
	MaxMs    int64
	ReqCount int64
}

func UpdateAgentHeartbeat(id string, s HeartbeatStats) error {
	return DB.Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"last_heartbeat":  time.Now().UTC(),
		"stats_allowed":   s.Allowed,
		"stats_blocked":   s.Blocked,
		"stats_avg_ms":    s.AvgMs,
		"stats_min_ms":    s.MinMs,
		"stats_max_ms":    s.MaxMs,
		"stats_req_count": s.ReqCount,
	}).Error
}

func UpdateAgentStatus(id, status, reason string) error {
	updates := map[string]any{"status": status}
	if reason != "" {
		updates["kill_reason"] = reason
	}
	return DB.Model(&Agent{}).Where("id = ?", id).Updates(updates).Error
}

func DeleteAgent(id string) error {
	return DB.Delete(&Agent{}, "id = ?", id).Error
}

func ListAgents(registrationFilter string) ([]Agent, error) {
	var agents []Agent
	q := DB.Order("registered_at DESC")
	if registrationFilter != "" {
		subq := DB.Model(&AgentProfile{}).Select("name").Where("registration = ?", registrationFilter)
		q = q.Where("agent_profile IN (?)", subq)
	}
	return agents, q.Find(&agents).Error
}

func CountHealthyAgents(registrationName string) (int64, error) {
	var count int64
	subq := DB.Model(&AgentProfile{}).Select("name").Where("registration = ?", registrationName)
	return count, DB.Model(&Agent{}).
		Where("agent_profile IN (?) AND status IN ?", subq, []string{"healthy", "stale"}).
		Count(&count).Error
}

// GetAgentRegistrationName returns the registration name for an agent by traversing Agent → AgentProfile → Registration.
func GetAgentRegistrationName(a *Agent) string {
	if a.AgentProfile == nil {
		return ""
	}
	profile, err := GetAgentProfile(*a.AgentProfile)
	if err != nil || profile.Registration == nil {
		return ""
	}
	return *profile.Registration
}

func GetAgentsForStaleCheck() ([]Agent, error) {
	var agents []Agent
	return agents, DB.Where("status IN ?", []string{"healthy", "stale"}).Find(&agents).Error
}

// --- Request Logs ---

type LogDomainStat struct {
	Domain           string
	RegistrationName string
	Action           string
	Count            int64
	LastSeen         string // raw string from MAX(created_at) — SQLite stores times as text
}

func QueryLogStats(registrationName string, since time.Time) ([]LogDomainStat, error) {
	var rows []LogDomainStat
	q := DB.Model(&RequestLog{}).
		Select("domain, registration_name, action, COUNT(*) as count, MAX(created_at) as last_seen").
		Where("created_at >= ?", since.UTC()).
		Group("domain, registration_name, action").
		Order("count DESC").
		Limit(500)
	if registrationName != "" {
		q = q.Where("registration_name = ?", registrationName)
	}
	return rows, q.Scan(&rows).Error
}

func InsertRequestLogs(logs []RequestLog) error {
	if len(logs) == 0 {
		return nil
	}
	return DB.CreateInBatches(logs, 100).Error
}

func QueryRequestLogs(registrationName, agentID, action, domain string, since time.Time, limit, offset int) ([]RequestLog, error) {
	var logs []RequestLog
	q := DB.Where("created_at >= ?", since.UTC()).Order("created_at DESC")
	if registrationName != "" {
		q = q.Where("registration_name = ?", registrationName)
	}
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if domain != "" {
		q = q.Where("domain LIKE ?", "%"+domain+"%")
	}
	if limit <= 0 {
		limit = 200
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	return logs, q.Limit(limit).Find(&logs).Error
}

func PurgeRequestLogs(olderThan time.Time) (int64, error) {
	result := DB.Where("created_at < ?", olderThan.UTC()).Delete(&RequestLog{})
	return result.RowsAffected, result.Error
}

// --- Metrics ---

func RecordMetric(agentID, registrationName string, s HeartbeatStats) error {
	return DB.Create(&Metric{
		AgentID:          agentID,
		RegistrationName: registrationName,
		Allowed:          s.Allowed,
		Blocked:          s.Blocked,
		AvgMs:            s.AvgMs,
		MinMs:            s.MinMs,
		MaxMs:            s.MaxMs,
		ReqCount:         s.ReqCount,
		CreatedAt:        time.Now().UTC().UTC(),
	}).Error
}

func GetMetrics(registrationName, agentID string, since time.Time, limit int) ([]Metric, error) {
	var metrics []Metric
	q := DB.Where("created_at >= ?", since.UTC()).Order("created_at DESC")
	if registrationName != "" {
		q = q.Where("registration_name = ?", registrationName)
	}
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	return metrics, q.Find(&metrics).Error
}

// --- Audit Events ---

func InsertAuditEvent(eventType, data string) error {
	return DB.Create(&AuditEvent{
		EventType: eventType,
		Data:      data,
		CreatedAt: time.Now().UTC().UTC(),
	}).Error
}

func QueryAuditEvents(since time.Time, limit int) ([]AuditEvent, error) {
	var events []AuditEvent
	q := DB.Where("created_at >= ?", since.UTC()).Order("created_at DESC")
	if limit <= 0 {
		limit = 50
	}
	return events, q.Limit(limit).Find(&events).Error
}

func PurgeAuditEvents(olderThan time.Time) (int64, error) {
	result := DB.Where("created_at < ?", olderThan.UTC()).Delete(&AuditEvent{})
	return result.RowsAffected, result.Error
}

func PurgeMetrics(olderThan time.Time) (int64, error) {
	result := DB.Where("created_at < ?", olderThan.UTC()).Delete(&Metric{})
	return result.RowsAffected, result.Error
}

// --- Cron Jobs ---

func CreateCronJob(job *CronJob) error {
	return DB.Create(job).Error
}

func GetCronJob(id uint) (*CronJob, error) {
	var job CronJob
	return &job, DB.First(&job, id).Error
}

func UpdateCronJob(id uint, updates map[string]any) error {
	updates["updated_at"] = time.Now().UTC()
	return DB.Model(&CronJob{}).Where("id = ?", id).Updates(updates).Error
}

func DeleteCronJob(id uint) error {
	return DB.Delete(&CronJob{}, id).Error
}

func ListCronJobs(profileName string) ([]CronJob, error) {
	var jobs []CronJob
	q := DB.Order("created_at DESC")
	if profileName != "" {
		q = q.Where("agent_profile_name = ?", profileName)
	}
	return jobs, q.Find(&jobs).Error
}

func ListCronJobsByRegistration(registrationName string) ([]CronJob, error) {
	var jobs []CronJob
	subq := DB.Model(&AgentProfile{}).Select("name").Where("registration = ?", registrationName)
	return jobs, DB.Where("registration_name = ? OR (registration_name = '' AND agent_profile_name IN (?))", registrationName, subq).Order("created_at DESC").Find(&jobs).Error
}

func ListEnabledCronJobs() ([]CronJob, error) {
	var jobs []CronJob
	return jobs, DB.Where("enabled = ?", true).Find(&jobs).Error
}

func ListCronJobsByProfile(profileName string) ([]CronJob, error) {
	var jobs []CronJob
	return jobs, DB.Where("agent_profile_name = ?", profileName).Find(&jobs).Error
}

// --- Cron Executions ---

func InsertCronExecution(exec *CronExecution) error {
	return DB.Create(exec).Error
}

func ListCronExecutions(cronJobID uint, limit int) ([]CronExecution, error) {
	var execs []CronExecution
	if limit <= 0 {
		limit = 50
	}
	return execs, DB.Where("cron_job_id = ?", cronJobID).
		Order("created_at DESC").Limit(limit).Find(&execs).Error
}

func PurgeCronExecutions(olderThan time.Time) (int64, error) {
	result := DB.Where("created_at < ?", olderThan.UTC()).Delete(&CronExecution{})
	return result.RowsAffected, result.Error
}
