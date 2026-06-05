package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"control-plane/internal/auth"
	"control-plane/internal/database"
)

type a2aRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type a2aResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *a2aError `json:"error,omitempty"`
}

type a2aError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type a2aMessage struct {
	Kind      string         `json:"kind"`
	MessageID string         `json:"messageId"`
	Role      string         `json:"role"`
	Parts     []a2aPart      `json:"parts"`
	ContextID string         `json:"contextId,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type a2aPart struct {
	Kind string       `json:"kind"`
	Text string       `json:"text,omitempty"`
	File *a2aFilePart `json:"file,omitempty"`
}

// a2aFilePart is an A2A FilePart. Attachments are uploaded to the control plane
// first (POST /uploads) and referenced here by uri (clawmatrix://uploads/<id>).
type a2aFilePart struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`   // clawmatrix://uploads/<id>
	Bytes    string `json:"bytes,omitempty"` // inline base64 — not supported; upload instead
}

// uploadURIPrefix is the scheme a FilePart.uri uses to reference a stored upload.
const uploadURIPrefix = "clawmatrix://uploads/"

type a2aTaskStatus struct {
	State     string      `json:"state"`
	Message   *a2aMessage `json:"message,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}

type a2aArtifact struct {
	ArtifactID string         `json:"artifactId"`
	Name       string         `json:"name,omitempty"`
	Parts      []a2aPart      `json:"parts,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type a2aTask struct {
	Kind      string         `json:"kind"`
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"`
	Status    a2aTaskStatus  `json:"status"`
	History   []a2aMessage   `json:"history,omitempty"`
	Artifacts []a2aArtifact  `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type a2aAuthContext struct {
	kind          string
	registration  *database.Registration
	user          *database.User
	agent         *database.Agent
	sourceProfile string
}

func (h *Handlers) AgentCard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("agent")
	if name == "" {
		name = r.URL.Query().Get("agent")
	}
	if name == "" {
		respond(w, 400, J{"error": "agent required"})
		return
	}
	profile, err := database.GetAgentProfile(name)
	if err != nil {
		respond(w, 404, J{"error": "agent profile not found"})
		return
	}
	desc := profile.Description
	if desc == "" {
		desc = fmt.Sprintf("%s agent profile", profile.Name)
	}
	respond(w, 200, J{
		"name":               profile.Name,
		"description":        desc,
		"url":                publicBaseURL(r) + "/a2a/" + profile.Name,
		"version":            "0.1.0",
		"defaultInputModes":  []string{"text/plain"},
		"defaultOutputModes": []string{"text/plain"},
		"capabilities": J{
			"streaming":              false,
			"pushNotifications":      false,
			"stateTransitionHistory": true,
		},
		"securitySchemes": J{
			"bearer": J{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
		},
		"security": []J{{"bearer": []any{}}},
		"skills": []J{{
			"id":          "delegate",
			"name":        "Delegate work to " + profile.Name,
			"description": "Submit tasks to the " + profile.Name + " agent.",
		}},
	})
}

func (h *Handlers) A2A(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("agent")
	if target == "" {
		writeA2A(w, nil, nil, -32602, "target agent required", nil)
		return
	}
	authCtx, errResp := h.a2aAuth(r)
	if errResp != nil {
		writeA2A(w, nil, nil, errResp.Code, errResp.Message, errResp.Data)
		return
	}

	var req a2aRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeA2A(w, nil, nil, -32700, "parse error", nil)
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		writeA2A(w, req.ID, nil, -32600, "invalid JSON-RPC version", nil)
		return
	}

	switch req.Method {
	case "message/send":
		h.a2aMessageSend(w, r, req, authCtx, target)
	case "tasks/get":
		h.a2aTasksGet(w, req, authCtx)
	case "tasks/cancel":
		h.a2aTasksCancel(w, req, authCtx)
	case "message/stream", "tasks/resubscribe":
		writeA2A(w, req.ID, nil, -32004, "unsupported operation", J{"method": req.Method})
	default:
		writeA2A(w, req.ID, nil, -32601, "method not found", J{"method": req.Method})
	}
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	authCtx, errResp := h.a2aAuth(r)
	if errResp != nil {
		respond(w, 401, J{"error": errResp.Message})
		return
	}
	task, err := database.GetA2ATask(r.PathValue("id"))
	if err != nil || !h.canViewA2ATask(authCtx, task) {
		respond(w, 404, J{"error": "task not found"})
		return
	}
	respond(w, 200, taskToA2A(task))
}

func (h *Handlers) a2aMessageSend(w http.ResponseWriter, r *http.Request, req a2aRequest, authCtx *a2aAuthContext, target string) {
	var params struct {
		Message  a2aMessage     `json:"message"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params.Message.Parts) == 0 {
		writeA2A(w, req.ID, nil, -32602, "invalid params", nil)
		return
	}
	if !h.canSendToTarget(authCtx, target) {
		writeA2A(w, req.ID, nil, -32001, "not authorized for target", J{"target": target})
		return
	}

	text := strings.TrimSpace(messageText(params.Message))
	attachBlock, aerr := h.resolveAttachments(authCtx, params.Message)
	if aerr != nil {
		writeA2A(w, req.ID, nil, -32602, aerr.Error(), nil)
		return
	}
	if attachBlock != "" {
		text = strings.TrimSpace(text + "\n\n" + attachBlock)
	}
	if text == "" {
		writeA2A(w, req.ID, nil, -32602, "message text or an attachment is required", nil)
		return
	}

	targetAgent, targetMeta, err := healthyTargetAgent(target)
	if err != nil {
		writeA2A(w, req.ID, nil, -32002, err.Error(), J{"target": target})
		return
	}

	cmMeta := mapStringAny(params.Metadata["clawmatrix"])
	if len(cmMeta) == 0 {
		cmMeta = mapStringAny(params.Message.Metadata["clawmatrix"])
	}
	async, _ := cmMeta["async"].(bool)
	sessionHint, _ := cmMeta["session"].(string)
	source := h.a2aSourceName(authCtx)

	contextID := params.Message.ContextID
	if contextID == "" {
		contextID = "ctx_" + randomID(12)
	}
	runtimeSession := runtimeSessionName(sessionPrefix(authCtx.kind), source, sessionHint, contextID)
	taskID := "task_" + randomID(12)
	now := time.Now().UTC()
	targetRunner, _ := targetMeta["runner"].(string)
	metadata := J{"clawmatrix": J{
		"target":         target,
		"source":         source,
		"session":        runtimeSession,
		"statusUrl":      "/tasks/" + taskID,
		"targetAgentId":  targetAgent.ID,
		"runtimeRunner":  targetRunner,
		"targetChatUrl":  targetMeta["chatUrl"],
		"requestedAsync": async,
	}}
	history := []a2aMessage{ensureMessageDefaults(params.Message, contextID)}
	historyJSON, _ := json.Marshal(history)
	metaJSON, _ := json.Marshal(metadata)
	statusMsg := statusMessage("Task submitted.")
	statusMsgJSON, _ := json.Marshal(statusMsg)

	task := &database.A2ATask{
		ID:              taskID,
		ContextID:       contextID,
		RuntimeSession:  runtimeSession,
		RuntimeRunner:   targetRunner,
		StatusState:     "submitted",
		StatusMessage:   string(statusMsgJSON),
		StatusTimestamp: now,
		History:         string(historyJSON),
		Artifacts:       "[]",
		Metadata:        string(metaJSON),
		SourceAgentID:   h.a2aSourceAgentID(authCtx),
		SourceProfile:   source,
		TargetAgentID:   targetAgent.ID,
		TargetProfile:   target,
	}
	if err := database.CreateA2ATask(task); err != nil {
		writeA2A(w, req.ID, nil, -32000, "failed to create task", nil)
		return
	}

	run := func() {
		h.runA2ATask(taskID, targetAgent, text, runtimeSession)
	}
	if async {
		go run()
		writeA2A(w, req.ID, taskToA2A(task), 0, "", nil)
		return
	}
	run()
	completed, _ := database.GetA2ATask(taskID)
	writeA2A(w, req.ID, taskToA2A(completed), 0, "", nil)
}

func (h *Handlers) a2aTasksGet(w http.ResponseWriter, req a2aRequest, authCtx *a2aAuthContext) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
		writeA2A(w, req.ID, nil, -32602, "task id required", nil)
		return
	}
	task, err := database.GetA2ATask(params.ID)
	if err != nil || !h.canViewA2ATask(authCtx, task) {
		writeA2A(w, req.ID, nil, -32004, "task not found", nil)
		return
	}
	writeA2A(w, req.ID, taskToA2A(task), 0, "", nil)
}

func (h *Handlers) a2aTasksCancel(w http.ResponseWriter, req a2aRequest, authCtx *a2aAuthContext) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
		writeA2A(w, req.ID, nil, -32602, "task id required", nil)
		return
	}
	task, err := database.GetA2ATask(params.ID)
	if err != nil || !h.canViewA2ATask(authCtx, task) {
		writeA2A(w, req.ID, nil, -32004, "task not found", nil)
		return
	}
	if task.StatusState == "completed" || task.StatusState == "failed" || task.StatusState == "canceled" {
		writeA2A(w, req.ID, taskToA2A(task), 0, "", nil)
		return
	}
	now := time.Now().UTC()
	msg := statusMessage("Task canceled.")
	msgJSON, _ := json.Marshal(msg)
	database.UpdateA2ATask(task.ID, map[string]any{
		"status_state":     "canceled",
		"status_message":   string(msgJSON),
		"status_timestamp": now,
	})
	updated, _ := database.GetA2ATask(task.ID)
	writeA2A(w, req.ID, taskToA2A(updated), 0, "", nil)
}

func (h *Handlers) runA2ATask(taskID string, target *database.Agent, message, session string) {
	now := time.Now().UTC()
	msg := statusMessage("Task started.")
	msgJSON, _ := json.Marshal(msg)
	database.UpdateA2ATask(taskID, map[string]any{
		"status_state":     "working",
		"status_message":   string(msgJSON),
		"status_timestamp": now,
	})
	h.broadcastTask(taskID, "working")

	response, err := callTargetAgent(target, message, session)
	task, _ := database.GetA2ATask(taskID)
	if task == nil {
		return
	}
	var history []a2aMessage
	json.Unmarshal([]byte(task.History), &history)
	var artifacts []a2aArtifact
	json.Unmarshal([]byte(task.Artifacts), &artifacts)
	state := "completed"
	statusText := "Task completed."
	if err != nil {
		state = "failed"
		statusText = err.Error()
	}
	if strings.TrimSpace(response) != "" {
		assistantMsg := a2aMessage{
			Kind:      "message",
			MessageID: "msg_" + randomID(12),
			Role:      "agent",
			ContextID: task.ContextID,
			Parts:     []a2aPart{{Kind: "text", Text: response}},
		}
		history = append(history, assistantMsg)
		artifacts = append(artifacts, a2aArtifact{
			ArtifactID: "artifact_" + randomID(8),
			Name:       "final-response",
			Parts:      []a2aPart{{Kind: "text", Text: response}},
		})
	}
	if task.RuntimeSession != "" {
		artifacts = append(artifacts, a2aArtifact{
			ArtifactID: "artifact_" + randomID(8),
			Name:       "runtime-session",
			Metadata: map[string]any{
				"clawmatrix": map[string]any{
					"session": task.RuntimeSession,
					"target":  task.TargetProfile,
				},
			},
		})
	}
	historyJSON, _ := json.Marshal(history)
	artifactsJSON, _ := json.Marshal(artifacts)
	status := statusMessage(statusText)
	statusJSON, _ := json.Marshal(status)
	database.UpdateA2ATask(taskID, map[string]any{
		"status_state":     state,
		"status_message":   string(statusJSON),
		"status_timestamp": time.Now().UTC(),
		"history":          string(historyJSON),
		"artifacts":        string(artifactsJSON),
	})
	h.broadcastTask(taskID, state)
}

// broadcastTask pushes a task state change over the SSE hub so clients can await
// completion (or stream progress) instead of polling.
func (h *Handlers) broadcastTask(taskID, state string) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(Event{Type: "task:updated", Data: J{"id": taskID, "state": state}})
}

func (h *Handlers) a2aAuth(r *http.Request) (*a2aAuthContext, *a2aError) {
	tok := bearer(r)
	if tok == "" {
		return nil, &a2aError{Code: -32001, Message: "unauthorized"}
	}
	if reg, err := database.GetRegistrationByToken(tok); err == nil && reg.Name != "" {
		return &a2aAuthContext{kind: "registration", registration: reg, sourceProfile: r.Header.Get("X-Clutch-Agent")}, nil
	}
	if agent, err := database.GetAgentByToken(tok); err == nil && agent.ID != "" {
		source := ""
		if agent.AgentProfile != nil {
			source = *agent.AgentProfile
		}
		return &a2aAuthContext{kind: "agent", agent: agent, sourceProfile: source}, nil
	}
	if claims, err := auth.Verify(tok); err == nil {
		if u, err := database.GetUserByID(claims.UserID); err == nil {
			return &a2aAuthContext{kind: "user", user: u}, nil
		}
	}
	if u, err := database.GetUserByToken(tok); err == nil && u != nil {
		return &a2aAuthContext{kind: "user", user: u}, nil
	}
	return nil, &a2aError{Code: -32001, Message: "unauthorized"}
}

func (h *Handlers) canSendToTarget(authCtx *a2aAuthContext, target string) bool {
	switch authCtx.kind {
	case "user":
		return database.UserHasProfilePerm(authCtx.user.ID, target, database.PermChatWithAgents)
	case "agent":
		return authCtx.sourceProfile != "" && database.HasConnection(authCtx.sourceProfile, target)
	case "registration":
		if authCtx.sourceProfile != "" {
			return database.HasConnection(authCtx.sourceProfile, target)
		}
		agents, _ := database.ListAgents(authCtx.registration.Name)
		for _, a := range agents {
			if a.AgentProfile != nil && database.HasConnection(*a.AgentProfile, target) {
				return true
			}
		}
	}
	return false
}

func (h *Handlers) canViewA2ATask(authCtx *a2aAuthContext, task *database.A2ATask) bool {
	if task == nil {
		return false
	}
	switch authCtx.kind {
	case "user":
		return database.UserHasProfilePerm(authCtx.user.ID, task.TargetProfile, database.PermViewAgents) ||
			database.UserHasProfilePerm(authCtx.user.ID, task.SourceProfile, database.PermViewAgents)
	case "agent":
		return authCtx.agent.ID == task.SourceAgentID || authCtx.agent.ID == task.TargetAgentID
	case "registration":
		regAgents, _ := database.ListAgents(authCtx.registration.Name)
		for _, a := range regAgents {
			if a.ID == task.SourceAgentID || a.ID == task.TargetAgentID {
				return true
			}
			if a.AgentProfile != nil && (*a.AgentProfile == task.SourceProfile || *a.AgentProfile == task.TargetProfile) {
				return true
			}
		}
	}
	return false
}

func (h *Handlers) a2aSourceName(authCtx *a2aAuthContext) string {
	if authCtx.sourceProfile != "" {
		return authCtx.sourceProfile
	}
	if authCtx.registration != nil {
		return authCtx.registration.Name
	}
	if authCtx.user != nil {
		return authCtx.user.Username
	}
	return "unknown"
}

func (h *Handlers) a2aSourceAgentID(authCtx *a2aAuthContext) string {
	if authCtx.agent != nil {
		return authCtx.agent.ID
	}
	return ""
}

func healthyTargetAgent(target string) (*database.Agent, map[string]any, error) {
	agents, err := database.GetAgentsByName(target)
	if err != nil || len(agents) == 0 {
		return nil, nil, fmt.Errorf("no agent found with name: %s", target)
	}
	for i := range agents {
		if agents[i].Status != "healthy" {
			continue
		}
		var meta map[string]any
		json.Unmarshal([]byte(agents[i].Meta), &meta)
		if chatURL, _ := meta["chatUrl"].(string); chatURL != "" {
			return &agents[i], meta, nil
		}
	}
	return nil, nil, fmt.Errorf("no healthy agent available for %s", target)
}

func callTargetAgent(agent *database.Agent, message, session string) (string, error) {
	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	chatURL, _ := meta["chatUrl"].(string)
	payload := J{"message": message, "session": session}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(context.Background(), "POST", chatURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+agent.Token)
	resp, err := chatClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("target agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("target agent returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return strings.TrimSpace(string(raw)), nil
	}
	if out.Error != "" {
		return out.Response, errors.New(out.Error)
	}
	return out.Response, nil
}

func taskToA2A(task *database.A2ATask) a2aTask {
	if task == nil {
		return a2aTask{}
	}
	var history []a2aMessage
	var artifacts []a2aArtifact
	var metadata map[string]any
	var statusMsg a2aMessage
	json.Unmarshal([]byte(task.History), &history)
	json.Unmarshal([]byte(task.Artifacts), &artifacts)
	json.Unmarshal([]byte(task.Metadata), &metadata)
	json.Unmarshal([]byte(task.StatusMessage), &statusMsg)
	status := a2aTaskStatus{
		State:     task.StatusState,
		Timestamp: task.StatusTimestamp.UTC().Format(time.RFC3339),
	}
	if len(statusMsg.Parts) > 0 {
		status.Message = &statusMsg
	}
	return a2aTask{
		Kind:      "task",
		ID:        task.ID,
		ContextID: task.ContextID,
		Status:    status,
		History:   history,
		Artifacts: artifacts,
		Metadata:  metadata,
	}
}

func writeA2A(w http.ResponseWriter, id any, result any, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	resp := a2aResponse{JSONRPC: "2.0", ID: id}
	if code != 0 {
		resp.Error = &a2aError{Code: code, Message: msg, Data: data}
	} else {
		resp.Result = result
	}
	json.NewEncoder(w).Encode(resp)
}

func messageText(msg a2aMessage) string {
	var parts []string
	for _, p := range msg.Parts {
		if p.Kind == "text" && strings.TrimSpace(p.Text) != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// resolveAttachments turns FilePart references into a human/agent-readable
// "[Attachments]" block carrying a signed, time-limited download link per file.
// The agent fetches these with its own tools; clutch/runners stay text-only.
// Users may only attach uploads they own.
func (h *Handlers) resolveAttachments(authCtx *a2aAuthContext, msg a2aMessage) (string, error) {
	var refs []uploadRef
	for _, p := range msg.Parts {
		if p.Kind != "file" || p.File == nil {
			continue
		}
		f := p.File
		if f.URI == "" {
			if f.Bytes != "" {
				return "", fmt.Errorf("inline attachment bytes are not supported — upload the file via POST /uploads first, then reference it by uri")
			}
			return "", fmt.Errorf("attachment is missing a uri")
		}
		refs = append(refs, uploadRef{Name: f.Name, URI: f.URI})
	}
	// User senders may only attach their own uploads; agent/registration senders bypass.
	userID := uint(0)
	admin := true
	if authCtx.kind == "user" {
		userID = authCtx.user.ID
		admin = isAdmin(authCtx.user)
	}
	return h.attachmentBlock(userID, admin, refs)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func ensureMessageDefaults(msg a2aMessage, contextID string) a2aMessage {
	if msg.Kind == "" {
		msg.Kind = "message"
	}
	if msg.MessageID == "" {
		msg.MessageID = "msg_" + randomID(12)
	}
	if msg.Role == "" {
		msg.Role = "user"
	}
	msg.ContextID = contextID
	return msg
}

func statusMessage(text string) a2aMessage {
	return a2aMessage{
		Kind:      "message",
		MessageID: "msg_" + randomID(12),
		Role:      "agent",
		Parts:     []a2aPart{{Kind: "text", Text: text}},
	}
}

// runtimeSessionName builds the runtime session id for a message. The prefix
// reflects who is asking: "delegate" only for agent→agent hand-offs; "ask" for a
// human/CLI (user) calling an agent directly.
func runtimeSessionName(prefix, source, hint, contextID string) string {
	if hint == "" {
		hint = contextID
	}
	if source == "" {
		return "a2a:" + hint
	}
	return prefix + ":" + source + ":" + hint
}

// sessionPrefix picks the session-name prefix from the caller kind. A user (PAT/
// JWT, e.g. via wealer or the UI) asking an agent is an "ask"; an agent or its
// clutch registration reaching another agent is a "delegate".
func sessionPrefix(kind string) string {
	if kind == "user" {
		return "ask"
	}
	return "delegate"
}

func mapStringAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func publicBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}
