package clutch

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func CpDo(method, path string, body any) (*http.Response, error) {
	return CpDoWithToken(method, path, body, CpToken)
}

func CpDoWithToken(method, path string, body any, token string) (*http.Response, error) {
	return cpDoWithTokenAndTimeout(method, path, body, token, 10*time.Second)
}

// CpDoLong is like CpDo but with a long timeout for agent chat/delegate calls.
func CpDoLong(method, path string, body any) (*http.Response, error) {
	return cpDoWithTokenAndTimeout(method, path, body, CpToken, 5*time.Minute)
}

func cpDoWithTokenAndTimeout(method, path string, body any, token string, timeout time.Duration) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, CpURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Timeout: timeout}).Do(req)
}

type logEntry struct {
	Time     string `json:"ts"`
	Action   string `json:"action"`
	Method   string `json:"method"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Status   int    `json:"status"`
	Duration int64  `json:"duration_ms,omitempty"`
}

func LogReq(action, method, domain, path string, d time.Duration, status int) {
	ts := time.Now().UTC().Format(time.RFC3339)
	ms := d.Milliseconds()

	if CpURL != "" {
		bufferLog(map[string]any{
			"domain":    domain,
			"method":    method,
			"path":      path,
			"action":    strings.ToLower(action),
			"status":    status,
			"latencyMs": ms,
			"ts":        ts,
		})
	}

	if action == "ALLOWED" && !LogAllowed {
		return
	}
	if action == "BLOCKED" && !LogBlocked {
		return
	}
	e := logEntry{
		Time:   ts,
		Action: action,
		Method: method,
		Domain: domain,
		Path:   path,
		Status: status,
	}
	if ms > 0 {
		e.Duration = ms
	}
	b, _ := json.Marshal(e)
	log.Println(string(b))
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	b, _ := json.Marshal(v)
	w.WriteHeader(code)
	w.Write(b)
}
