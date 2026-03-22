package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(driver, uri string) error {
	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		dialector = sqlite.Open(uri)
	case "postgres":
		dialector = postgres.Open(uri)
	default:
		return fmt.Errorf("unsupported DB driver: %s", driver)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}

	if driver == "sqlite" {
		DB.Exec("PRAGMA journal_mode=WAL")
	}

	if err := DB.AutoMigrate(
		&Registration{}, &Connection{}, &AgentProfile{}, &Agent{},
		&RequestLog{}, &Metric{}, &AuditEvent{}, &CronJob{}, &CronExecution{}, &AcmeCache{},
		&User{}, &Role{}, &RolePermission{}, &AgentProfileACL{}, &UserIdentity{},
	); err != nil {
		return err
	}

	SeedRoles()

	log.Printf("database ready: %s (%s)", uri, driver)
	return nil
}

func Bootstrap(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	var bootstrap struct {
		Registrations []struct {
			Name       string   `json:"name"`
			Token      string   `json:"token"`
			Allowlist  []string `json:"allowlist"`
			TTLMinutes int      `json:"ttlMinutes"`
		} `json:"registrations"`
		Connections []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		log.Fatalf("bootstrap parse: %v", err)
	}

	for _, e := range bootstrap.Registrations {
		var existing Registration
		if DB.Where("name = ?", e.Name).First(&existing).Error == nil {
			continue // already exists, skip
		}

		allowlistJSON, _ := json.Marshal(e.Allowlist)
		t := Registration{
			Name:            e.Name,
			TokenHash:       HashToken(e.Token),
			EgressAllowlist: string(allowlistJSON),
			TTLMinutes:      e.TTLMinutes,
		}
		DB.Create(&t)
		log.Printf("  bootstrap: %s (ttl %dm)", e.Name, e.TTLMinutes)
	}

	for _, c := range bootstrap.Connections {
		if !HasConnection(c.Source, c.Target) {
			CreateConnection(c.Source, c.Target)
			log.Printf("  bootstrap connection: %s -> %s", c.Source, c.Target)
		}
	}
}
