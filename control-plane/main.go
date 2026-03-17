package main

import (
	"fmt"
	"log"
	"net/http"

	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/acme/autocert"

	"control-plane/internal/api"
	"control-plane/internal/config"
	cronpkg "control-plane/internal/cron"
	"control-plane/internal/database"
	"control-plane/internal/worker"
)

func main() {
	cfg := config.Load()
	if cfg.AdminToken == "" {
		log.Fatal("ADMIN_TOKEN env required")
	}

	if err := database.Init(cfg.DB, cfg.DBURI); err != nil {
		log.Fatalf("database: %v", err)
	}

	if cfg.BootstrapConfig != "" {
		database.Bootstrap(cfg.BootstrapConfig)
	}

	hub := api.NewHub()
	scheduler := cronpkg.NewScheduler(hub)
	scheduler.Start()
	defer scheduler.Stop()

	go worker.StaleLoop(hub)

	router := api.NewRouter(cfg.AdminToken, hub, scheduler)
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
		// port 80: ACME HTTP-01 challenge + redirect to HTTPS
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
