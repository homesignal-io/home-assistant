package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/platform/database"
)

const (
	smokeAccountID = "acct_staging_smoke"
	smokeSiteID    = "site_staging_smoke"
)

func main() {
	var (
		mode        = flag.String("mode", "", "one of: seed-telemetry-device, cleanup-telemetry-device")
		deviceID    = flag.String("device-id", "", "staging smoke device ID")
		databaseURL = flag.String("database-url", "", "PostgreSQL connection URL; defaults to HOMESIGNAL_DATABASE_URL or DATABASE_URL")
		timeout     = flag.Duration("timeout", 30*time.Second, "fixture command timeout")
	)
	flag.Parse()

	if strings.TrimSpace(*deviceID) == "" {
		exitf(2, "-device-id is required")
	}
	if !strings.HasPrefix(*deviceID, "dev_smoke-") {
		exitf(2, "refusing to mutate non-smoke device id %q", *deviceID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfg := database.LoadConfigFromEnv()
	if *databaseURL != "" {
		cfg.URL = *databaseURL
	}
	db, err := database.Open(ctx, cfg)
	if err != nil {
		exitf(1, "%v", err)
	}
	defer db.Close()

	switch *mode {
	case "seed-telemetry-device":
		if err := seedTelemetryDevice(ctx, db, *deviceID); err != nil {
			exitf(1, "seed telemetry fixture: %v", err)
		}
		fmt.Printf("Seeded telemetry fixture %s\n", *deviceID)
	case "cleanup-telemetry-device":
		if err := cleanupTelemetryDevice(ctx, db, *deviceID); err != nil {
			exitf(1, "cleanup telemetry fixture: %v", err)
		}
		fmt.Printf("Cleaned telemetry fixture %s\n", *deviceID)
	default:
		exitf(2, "unsupported -mode %q", *mode)
	}
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func seedTelemetryDevice(ctx context.Context, db execer, deviceID string) error {
	if _, err := db.ExecContext(ctx, `
INSERT INTO accounts (account_id, display_name, status)
VALUES ($1, 'Staging Smoke Account', 'active')
ON CONFLICT (account_id) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  updated_at = now()
`, smokeAccountID); err != nil {
		return fmt.Errorf("upsert smoke account: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sites (site_id, account_id, display_name, time_zone)
VALUES ($1, $2, 'Staging Smoke Site', 'America/New_York')
ON CONFLICT (site_id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  display_name = EXCLUDED.display_name,
  updated_at = now()
`, smokeSiteID, smokeAccountID); err != nil {
		return fmt.Errorf("upsert smoke site: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO devices (
  device_id,
  account_id,
  site_id,
  iot_thing_name,
  lifecycle_status,
  claim_state,
  claimed_at
)
VALUES ($1, $2, $3, $1, 'registered', 'CLAIMED', now())
ON CONFLICT (device_id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  site_id = EXCLUDED.site_id,
  iot_thing_name = EXCLUDED.iot_thing_name,
  updated_at = now()
`, deviceID, smokeAccountID, smokeSiteID); err != nil {
		return fmt.Errorf("upsert smoke device: %w", err)
	}
	return nil
}

func cleanupTelemetryDevice(ctx context.Context, db execer, deviceID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM devices WHERE device_id = $1`, deviceID); err != nil {
		return fmt.Errorf("delete smoke device: %w", err)
	}
	return nil
}

func exitf(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
