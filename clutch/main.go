package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clutch "clutch/internal"
)

// Set at build time: go build -ldflags "-X main.gatewayVersion=2.0.0"
var gatewayVersion = "dev"

func runCLI() {
	if len(os.Args) < 2 {
		return // not a CLI invocation
	}

	gatewayURL := "http://127.0.0.1:8080"
	if u := os.Getenv("CLUTCH_URL"); u != "" {
		gatewayURL = u
	}

	// Parse optional --name <agent> flag before the subcommand.
	agentName := os.Getenv("CLUTCH_NAME")
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--name" {
		agentName = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		return
	}
	sub := args[0]

	doReq := func(method, path, body string) {
		var req *http.Request
		if body != "" {
			req, _ = http.NewRequest(method, gatewayURL+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequest(method, gatewayURL+path, nil)
		}
		if agentName != "" {
			req.Header.Set("X-Clutch-Agent", agentName)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && method != http.MethodDelete {
			io.Copy(os.Stderr, resp.Body)
			fmt.Fprintln(os.Stderr)
			os.Exit(1)
		}
		io.Copy(os.Stdout, resp.Body)
		fmt.Println()
	}
	cliGet := func(path string) { doReq(http.MethodGet, path, "") }
	cliPost := func(path, body string) { doReq(http.MethodPost, path, body) }
	cliPut := func(path, body string) { doReq(http.MethodPut, path, body) }
	cliDelete := func(path string) { doReq(http.MethodDelete, path, "") }

	switch sub {
	case "connections":
		cliGet("/connections")
		os.Exit(0)

	case "delegate":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, `usage: clutch [--name <agent>] delegate <agent> "<message>" [session]`)
			os.Exit(1)
		}
		target, message := args[1], args[2]
		payload := map[string]string{"message": message}
		if len(args) > 3 {
			payload["session"] = args[3]
		}
		body, _ := json.Marshal(payload)
		cliPost("/delegate/"+target, string(body))
		os.Exit(0)

	case "crons":
		if len(args) < 2 {
			cliGet("/crons")
			os.Exit(0)
		}
		switch args[1] {
		case "create":
			if len(args) < 3 {
				fmt.Fprintln(os.Stderr, `usage: clutch [--name <agent>] crons create '<json>'`)
				os.Exit(1)
			}
			cliPost("/crons", args[2])
		case "update":
			if len(args) < 4 {
				fmt.Fprintln(os.Stderr, `usage: clutch [--name <agent>] crons update <id> '<json>'`)
				os.Exit(1)
			}
			cliPut("/crons/"+args[2], args[3])
		case "delete":
			if len(args) < 3 {
				fmt.Fprintln(os.Stderr, `usage: clutch [--name <agent>] crons delete <id>`)
				os.Exit(1)
			}
			cliDelete("/crons/" + args[2])
		default:
			fmt.Fprintf(os.Stderr, "unknown crons subcommand: %s\n", args[1])
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func main() {
	runCLI()

	listen := flag.String("listen", "127.0.0.1:8080", "proxy listen address")
	localFile := flag.String("allowlist", "", "local allowlist file (fallback if no control plane)")
	preferredID := flag.String("agent-id", "", "stable agent ID (or AGENT_ID env)")
	agentCmdFlag := flag.String("agent-cmd", "", "command to run for /ask endpoint (or AGENT_CMD env)")
	agentTimeoutFlag := flag.Duration("agent-timeout", 120*time.Second, "timeout for agent subprocess (or AGENT_TIMEOUT env)")
	runnerFlag := flag.String("runner", "", "runner type, e.g. 'picoclaw' or 'openclaw' (or RUNNER env)")
	workspaceFlag := flag.String("workspace", "", "workspace directory for /workspace endpoint (or WORKSPACE_PATH env)")
	sessionsFlag := flag.String("sessions", "", "sessions directory for /sessions endpoint (or SESSIONS_PATH env)")
	agentGatewayFlag := flag.String("agent-gateway", "", "agent gateway URL for openclaw HTTP forwarding (or AGENT_GATEWAY_URL env)")
	agentGatewayTokenFlag := flag.String("agent-gateway-token", "", "bearer token for agent gateway auth (or AGENT_GATEWAY_TOKEN env)")
	noSnifferFlag := flag.Bool("no-sniffer", false, "disable sniffer goroutine (or DISABLE_SNIFFER env)")
	flag.StringVar(&clutch.CpURL, "control-plane", "", "control plane URL (or CONTROL_PLANE_URL env)")
	flag.StringVar(&clutch.CpToken, "token", "", "registration token (or REGISTRATION_TOKEN env)")
	flag.BoolVar(&clutch.LogAllowed, "log-allowed", true, "log allowed requests")
	flag.BoolVar(&clutch.LogBlocked, "log-blocked", true, "log blocked requests")
	flag.Parse()

	if clutch.CpURL == "" {
		clutch.CpURL = os.Getenv("CONTROL_PLANE_URL")
	}
	if clutch.CpToken == "" {
		clutch.CpToken = os.Getenv("REGISTRATION_TOKEN")
	}

	if *preferredID == "" {
		*preferredID = os.Getenv("AGENT_ID")
	}
	clutch.PreferredAgentID = *preferredID
	clutch.PreferredAgentGroup = os.Getenv("AGENT_GROUP")

	clutch.AgentCmd = *agentCmdFlag
	if clutch.AgentCmd == "" {
		clutch.AgentCmd = os.Getenv("AGENT_CMD")
	}
	clutch.AgentTimeout = *agentTimeoutFlag
	if v := os.Getenv("AGENT_TIMEOUT"); v != "" && *agentCmdFlag == "" {
		if d, err := time.ParseDuration(v); err == nil {
			clutch.AgentTimeout = d
		}
	}
	clutch.Runner = *runnerFlag
	if clutch.Runner == "" {
		clutch.Runner = os.Getenv("RUNNER")
	}
	clutch.WorkspacePath = *workspaceFlag
	if clutch.WorkspacePath == "" {
		clutch.WorkspacePath = os.Getenv("WORKSPACE_PATH")
	}
	clutch.SessionsPath = *sessionsFlag
	if clutch.SessionsPath == "" {
		clutch.SessionsPath = os.Getenv("SESSIONS_PATH")
	}
	clutch.AgentGatewayURL = *agentGatewayFlag
	if clutch.AgentGatewayURL == "" {
		clutch.AgentGatewayURL = os.Getenv("AGENT_GATEWAY_URL")
	}
	clutch.AgentGatewayToken = *agentGatewayTokenFlag
	if clutch.AgentGatewayToken == "" {
		clutch.AgentGatewayToken = os.Getenv("AGENT_GATEWAY_TOKEN")
	}
	clutch.SnifferDisabled = *noSnifferFlag
	if !clutch.SnifferDisabled {
		clutch.SnifferDisabled = os.Getenv("DISABLE_SNIFFER") == "true"
	}

	clutch.ListenAddr = *listen
	clutch.HostBaseURL = os.Getenv("HOST_URL")
	if clutch.HostBaseURL == "" {
		addr := clutch.ListenAddr
		if strings.HasPrefix(addr, "0.0.0.0:") {
			addr = "localhost:" + strings.TrimPrefix(addr, "0.0.0.0:")
		} else if strings.HasPrefix(addr, "[::]:") {
			addr = "localhost:" + strings.TrimPrefix(addr, "[::]:")
		}
		clutch.HostBaseURL = "http://" + addr
	}

	clutch.GatewayVersion = gatewayVersion

	srv := &http.Server{
		Addr:    *listen,
		Handler: http.HandlerFunc(clutch.Handle),
	}

	// Start the HTTP server immediately so the port is reachable during
	// registration retries. Fatal to main if the port cannot be bound.
	srvErr := make(chan error, 1)
	go func() {
		log.Printf("clutch %s on %s", gatewayVersion, *listen)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			srvErr <- err
		}
	}()
	// Give the server a moment to bind; exit immediately on failure (e.g. port in use).
	select {
	case err := <-srvErr:
		log.Fatalf("server: %v", err)
	case <-time.After(500 * time.Millisecond):
		// bound successfully, continue
	}

	if clutch.CpURL != "" && clutch.CpToken != "" {
		clutch.Register()
	} else if *localFile != "" {
		clutch.LoadLocalAllowlist(*localFile)
	} else {
		log.Fatal("need --control-plane + --token, or --allowlist")
	}

	if clutch.CpURL != "" {
		go clutch.HeartbeatLoop()
		go clutch.ConfigPollLoop()
		go clutch.LogFlushLoop()
	}

	// Start sniffer goroutine (Linux only, skipped if disabled or no caps)
	if !clutch.SnifferDisabled {
		if err := clutch.StartSniffer(); err != nil {
			log.Printf("sniffer: %v", err)
		}
	} else {
		log.Printf("sniffer: disabled")
	}

	// Graceful shutdown
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		log.Println("shutting down")
		clutch.DeregisterAll()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if clutch.AgentGatewayURL != "" {
		log.Printf("openclaw gateway mode: %s (subprocess connects via WS)", clutch.AgentGatewayURL)
	} else if clutch.AgentCmd != "" {
		log.Printf("agent executor enabled: %q (timeout %s)", clutch.AgentCmd, clutch.AgentTimeout)
	}
	if len(clutch.RegisteredAgents) > 0 {
		for _, a := range clutch.RegisteredAgents {
			log.Printf("  agent: %s → %s (workspace=%s)", a.LocalID(), a.FullID(), a.Workspace())
		}
	}
	if clutch.WorkspacePath != "" {
		log.Printf("workspace browser enabled: %s", clutch.WorkspacePath)
	}
	if clutch.SessionsPath != "" {
		log.Printf("sessions browser enabled: %s", clutch.SessionsPath)
	}
	if clutch.CpURL != "" {
		log.Printf("cron management enabled: /crons")
	}

	select {}
}

// resolveURLHost replaces the hostname in rawURL with its resolved IP address.
func resolveURLHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = ""
	}
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	ip := addrs[0]
	if port != "" {
		u.Host = net.JoinHostPort(ip, port)
	} else {
		u.Host = ip
	}
	return u.String()
}
