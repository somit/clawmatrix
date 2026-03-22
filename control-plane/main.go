package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/acme/autocert"

	"control-plane/internal/api"
	"control-plane/internal/auth"
	"control-plane/internal/config"
	cronpkg "control-plane/internal/cron"
	"control-plane/internal/database"
	"control-plane/internal/worker"
)

func main() {
	cfg := config.Load()

	// CLI subcommands — connect to DB directly, no server needed
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "createadmin":
			runCreateAdmin(cfg)
			return
		case "listadmins":
			runListAdmins(cfg)
			return
		case "deleteadmin":
			runDeleteAdmin(cfg)
			return
		}
	}

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET env required")
	}

	if err := database.Init(cfg.DB, cfg.DBURI); err != nil {
		log.Fatalf("database: %v", err)
	}

	auth.Init(cfg.JWTSecret)

	if cfg.BootstrapConfig != "" {
		database.Bootstrap(cfg.BootstrapConfig)
	}

	hub := api.NewHub()
	scheduler := cronpkg.NewScheduler(hub)
	scheduler.Start()
	defer scheduler.Stop()

	go worker.StaleLoop(hub)

	oidcCfg, err := api.NewOIDCConfig(
		cfg.OIDCIssuerURL,
		cfg.OIDCClientID,
		cfg.OIDCClientSecret,
		cfg.OIDCRedirectBaseURL,
		cfg.OIDCButtonLabel,
	)
	if err != nil {
		log.Fatalf("OIDC init: %v", err)
	}
	if oidcCfg != nil {
		log.Printf("OIDC enabled (issuer: %s)", cfg.OIDCIssuerURL)
	}

	router := api.NewRouter(hub, scheduler, oidcCfg)
	fmt.Println(`
+--------------------+
|    ClawMatrix      |
|                    |
|  . . . . . . . .  |
|  . (\/) . (\/) .  |
|  . <..> . <..> .  |
|  .  /\  .  /\  .  |
|  . . . . . . . .  |
+--------------------+`)
	if cfg.TLSDomain != "" {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.TLSDomain),
			Cache:      database.DBCertCache{},
			Email:      cfg.TLSEmail,
		}
		go func() {
			log.Printf("ACME HTTP handler on :80")
			log.Fatal(http.ListenAndServe(":80", m.HTTPHandler(nil)))
		}()
		srv := &http.Server{
			Addr:      ":443",
			Handler:   router,
			TLSConfig: m.TLSConfig(),
		}
		log.Printf("listening on :443 (TLS, domain: %s)", cfg.TLSDomain)
		log.Fatal(srv.ListenAndServeTLS("", ""))
	} else {
		log.Printf("listening on %s", cfg.Listen)
		log.Fatal(http.ListenAndServe(cfg.Listen, router))
	}
}

func runCreateAdmin(cfg *config.Config) {
	fs := flag.NewFlagSet("createadmin", flag.ExitOnError)
	username := fs.String("username", "", "Admin username (required)")
	password := fs.String("password", "", "Admin password (required)")
	fs.Parse(os.Args[2:])

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: control-plane createadmin --username <name> --password <pass>")
		os.Exit(1)
	}

	if err := database.Init(cfg.DB, cfg.DBURI); err != nil {
		log.Fatalf("database: %v", err)
	}

	adminRoleID, err := database.AdminRoleID()
	if err != nil {
		log.Fatalf("admin role not found (database not initialized?): %v", err)
	}

	u, err := database.CreateUser(*username, *password, &adminRoleID, nil)
	if err != nil {
		log.Fatalf("create user: %v", err)
	}
	fmt.Printf("admin created: %s (id=%d)\n", u.Username, u.ID)
}

func runListAdmins(cfg *config.Config) {
	if err := database.Init(cfg.DB, cfg.DBURI); err != nil {
		log.Fatalf("database: %v", err)
	}

	users, err := database.ListUsers()
	if err != nil {
		log.Fatalf("list users: %v", err)
	}

	fmt.Printf("%-5s %-20s %-15s\n", "ID", "USERNAME", "SYSTEM ROLE")
	for _, u := range users {
		role := "-"
		if u.SystemRole != nil {
			role = u.SystemRole.Name
		}
		fmt.Printf("%-5d %-20s %-15s\n", u.ID, u.Username, role)
	}
}

func runDeleteAdmin(cfg *config.Config) {
	fs := flag.NewFlagSet("deleteadmin", flag.ExitOnError)
	username := fs.String("username", "", "Admin username (required)")
	fs.Parse(os.Args[2:])

	if *username == "" {
		fmt.Fprintln(os.Stderr, "usage: control-plane deleteadmin --username <name>")
		os.Exit(1)
	}

	if err := database.Init(cfg.DB, cfg.DBURI); err != nil {
		log.Fatalf("database: %v", err)
	}

	u, err := database.GetUserByUsername(*username)
	if err != nil {
		log.Fatalf("user not found: %v", err)
	}

	if err := database.DeleteUser(u.ID); err != nil {
		log.Fatalf("delete user: %v", err)
	}
	fmt.Printf("deleted: %s\n", u.Username)
}
