package ui

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
)

// Assets holds versioned URLs for all static files, providing cache-busting
// via content hashes computed from the embedded FS at startup.
type Assets struct {
	StyleCSS        string
	AppJS           string
	RegistrationsJS string
	TemplatesJS     string
	ConnectionsJS   string
	AgentsJS        string
	DashboardJS     string
	LogsJS          string
	CronsJS         string
	EventsJS        string
	ChatJS          string
	WorkspaceJS     string
	SessionsJS      string
	UsersJS         string
}

func computeAssets(fs embed.FS) Assets {
	hash := func(path string) string {
		f, err := fs.Open(path)
		if err != nil {
			return "0"
		}
		defer f.Close()
		h := sha256.New()
		io.Copy(h, f)
		return fmt.Sprintf("%x", h.Sum(nil))[:8]
	}
	v := func(path string) string {
		return "/ui/" + path + "?v=" + hash(path)
	}
	return Assets{
		StyleCSS:        v("style.css"),
		AppJS:           v("js/app.js"),
		RegistrationsJS: v("js/registrations.js"),
		TemplatesJS:     v("js/templates.js"),
		ConnectionsJS:   v("js/connections.js"),
		AgentsJS:        v("js/agents.js"),
		DashboardJS:     v("js/dashboard.js"),
		LogsJS:          v("js/logs.js"),
		CronsJS:         v("js/crons.js"),
		EventsJS:        v("js/events.js"),
		ChatJS:          v("js/chat.js"),
		WorkspaceJS:     v("js/workspace.js"),
		SessionsJS:      v("js/sessions.js"),
		UsersJS:         v("js/users.js"),
	}
}
