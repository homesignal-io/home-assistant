package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/platform/database"
)

const (
	stagingAccountID = "acct_staging_smoke"
	stagingSiteID    = "site_staging_smoke"
	stagingOwnerRole = "role_staging_owner"
)

var ownerPermissions = []string{
	"account:delete",
	"account:view",
	"agent:update",
	"alert:acknowledge",
	"alert:view",
	"alert_recipient:manage",
	"alert_recipient:view",
	"audit:view",
	"backup:trigger",
	"backup:view",
	"billing:manage",
	"billing:view",
	"device:diagnose",
	"device:refresh",
	"device:update",
	"device:view",
	"diagnostics:request",
	"member:invite",
	"member:manage_owner",
	"member:remove",
	"member:update_role",
	"member:view",
	"role:view",
	"site:create",
	"site:delete",
	"site:refresh",
	"site:update",
	"site:view",
	"telemetry:view",
	"topology:view",
	"update:view",
}

func main() {
	var (
		email       = flag.String("email", "", "portal user email")
		cognitoSub  = flag.String("cognito-sub", "", "Cognito subject for the portal user")
		displayName = flag.String("display-name", "", "portal user display name")
		databaseURL = flag.String("database-url", "", "PostgreSQL connection URL; defaults to HOMESIGNAL_DATABASE_URL or DATABASE_URL")
		timeout     = flag.Duration("timeout", 30*time.Second, "bootstrap timeout")
	)
	flag.Parse()

	normalizedEmail := strings.ToLower(strings.TrimSpace(*email))
	if normalizedEmail == "" {
		exitf(2, "-email is required")
	}
	if strings.TrimSpace(*cognitoSub) == "" {
		exitf(2, "-cognito-sub is required")
	}
	if strings.TrimSpace(*displayName) == "" {
		*displayName = normalizedEmail
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

	userID := stableUserID(normalizedEmail)
	if err := bootstrapPortalUser(ctx, db, portalUser{
		ID:              userID,
		CognitoSub:      strings.TrimSpace(*cognitoSub),
		Email:           strings.TrimSpace(*email),
		EmailNormalized: normalizedEmail,
		DisplayName:     strings.TrimSpace(*displayName),
	}); err != nil {
		exitf(1, "bootstrap staging portal user: %v", err)
	}
	fmt.Printf("Bootstrapped staging portal user %s as %s\n", normalizedEmail, userID)
}

type portalUser struct {
	ID              string
	CognitoSub      string
	Email           string
	EmailNormalized string
	DisplayName     string
}

func bootstrapPortalUser(ctx context.Context, db *sql.DB, user portalUser) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO accounts (account_id, display_name, status, account_type)
VALUES ($1, 'Staging Smoke Account', 'active', 'dealer')
ON CONFLICT (account_id) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  status = 'active',
  updated_at = now()
`, stagingAccountID); err != nil {
		return fmt.Errorf("upsert staging account: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO sites (site_id, account_id, display_name, time_zone, site_category)
VALUES ($1, $2, 'Staging Smoke Site', 'America/New_York', 'business')
ON CONFLICT (site_id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  display_name = EXCLUDED.display_name,
  updated_at = now()
`, stagingSiteID, stagingAccountID); err != nil {
		return fmt.Errorf("upsert staging site: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (user_id, cognito_sub, email, email_normalized, display_name, status)
VALUES ($1, $2, $3, $4, $5, 'active')
ON CONFLICT (user_id) DO UPDATE SET
  cognito_sub = EXCLUDED.cognito_sub,
  email = EXCLUDED.email,
  email_normalized = EXCLUDED.email_normalized,
  display_name = EXCLUDED.display_name,
  status = 'active',
  updated_at = now(),
  deleted_at = NULL
`, user.ID, user.CognitoSub, user.Email, user.EmailNormalized, user.DisplayName); err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	var ownerRoleID string
	if err := tx.QueryRowContext(ctx, `
WITH existing AS (
  SELECT role_id
  FROM roles
  WHERE account_id IS NULL
    AND name = 'owner'
  LIMIT 1
),
inserted AS (
  INSERT INTO roles (role_id, account_id, name, description, is_system)
  SELECT $1, NULL, 'owner', 'Staging owner role for first portal bootstrap.', true
  WHERE NOT EXISTS (SELECT 1 FROM existing)
  RETURNING role_id
)
SELECT role_id FROM inserted
UNION ALL
SELECT role_id FROM existing
LIMIT 1
`, stagingOwnerRole).Scan(&ownerRoleID); err != nil {
		return fmt.Errorf("resolve owner role: %w", err)
	}

	for _, permission := range ownerPermissions {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO role_permissions (role_id, action)
VALUES ($1, $2)
ON CONFLICT (role_id, action) DO NOTHING
`, ownerRoleID, permission); err != nil {
			return fmt.Errorf("upsert owner permission %s: %w", permission, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO account_memberships (account_id, user_id, role_id, status)
VALUES ($1, $2, $3, 'active')
ON CONFLICT (account_id, user_id) DO UPDATE SET
  role_id = EXCLUDED.role_id,
  status = 'active',
  removed_at = NULL,
  updated_at = now()
`, stagingAccountID, user.ID, ownerRoleID); err != nil {
		return fmt.Errorf("upsert account membership: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO site_relationships (
  site_id,
  account_id,
  relationship_type,
  status,
  created_by_user_id
)
SELECT $1, $2, 'owner', 'active', $3
WHERE NOT EXISTS (
  SELECT 1
  FROM site_relationships
  WHERE site_id = $1
    AND relationship_type = 'owner'
    AND status = 'active'
)
`, stagingSiteID, stagingAccountID, user.ID); err != nil {
		return fmt.Errorf("ensure owner site relationship: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func stableUserID(email string) string {
	sum := sha256.Sum256([]byte(email))
	return "user_staging_" + hex.EncodeToString(sum[:])[:16]
}

func exitf(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
