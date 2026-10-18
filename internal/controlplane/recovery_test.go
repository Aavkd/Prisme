package controlplane

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/operations"
	"prisme.local/platform/internal/store"
)

// This drill needs the isolated PostgreSQL/Restic test image. It destroys only
// databases created by this test, with generated names, never the source DSN.
func TestPhaseTenControlPlaneRestore(t *testing.T) {
	if os.Getenv("PRISME_TEST_RESTORE") != "1" {
		t.Skip("isolated restore drill required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	started := time.Now()
	u, err := url.Parse(os.Getenv("PRISME_TEST_DATABASE_URL"))
	if err != nil || u.Hostname() != "127.0.0.1" || u.User == nil {
		t.Fatal("isolated loopback database required")
	}
	admin, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	root := t.TempDir()
	pgEnv := append(os.Environ(), "PGHOST="+u.Hostname(), "PGPORT="+u.Port(), "PGUSER="+u.User.Username())
	password, _ := u.User.Password()
	pgEnv = append(pgEnv, "PGPASSWORD="+password)
	run := func(env []string, name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		// A failed process must not print connection strings or secret material.
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s failed (%T)", name, err)
		}
	}
	dbName := fmt.Sprintf("prisme_restore_%d", time.Now().UnixNano())
	newDB := func(name string) *sql.DB {
		t.Helper()
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`) })
		copy := *u
		copy.Path = "/" + name
		db, err := store.Open(ctx, copy.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	source := newDB(dbName)
	if err = store.Migrate(ctx, source); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, "bundle")
	if err = os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	master := make([]byte, 32)
	rand.Read(master)
	keyPath := filepath.Join(stage, "vault.key")
	if err = os.WriteFile(keyPath, master, 0600); err != nil {
		t.Fatal(err)
	}
	vault, err := operations.NewVault(source, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	principal := operations.Principal{Actor: "restore-fixture", Device: "fixture", Session: "fixture"}
	secret, err := vault.Put(ctx, principal, operations.Secret{Name: "restore-check", Scope: "control", Kind: "token", Destination: "fixture", ExpiresAt: time.Now().Add(time.Hour)}, []byte("private-restore-sentinel"))
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO sessions(token_hash,device_name,expires_at) VALUES('old-session','fixture',now()+interval '1 hour')`,
		`INSERT INTO passkeys(id,credential,name) VALUES('old-key','{}','fixture')`,
		`INSERT INTO recovery_codes(code_hash) VALUES('old-code')`,
		`INSERT INTO deployment_targets(id,manifest_hash,manifest) VALUES('fixture','hash','{}')`,
		`INSERT INTO deployment_jobs(id,target_id,request_key,request_hash,plan,state,expires_at) VALUES('pending','fixture','pending','hash','{}','ready',now()+interval '1 hour')`,
		`UPDATE automation_control SET stopped=false`,
		`INSERT INTO automation_schedules(id,document,next_at,enabled) VALUES('fixture','{"enabled":true}',now(),true)`,
		`INSERT INTO automation_runs(id,request_key,request_hash,target,state,document) VALUES('fixture','fixture','hash','fixture','running','{"state":"running","revision":1,"approved_step":2}')`,
		`INSERT INTO backup_policies(id,config_hash,project,environment,server,repository) VALUES('fixture','hash','fixture','test','fixture','fixture')`,
	} {
		if _, err = source.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	dump := filepath.Join(stage, "database.dump")
	run(pgEnv, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file", dump, "--dbname", dbName)
	// The bundle including the independent master key is encrypted by a separate
	// Restic recovery password, held outside the repository and PostgreSQL.
	recoveryPassword := filepath.Join(root, "restic-password")
	if err = os.WriteFile(recoveryPassword, []byte(strings.Repeat("r", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	resticEnv := append(os.Environ(), "RESTIC_REPOSITORY="+filepath.Join(root, "repository"), "RESTIC_PASSWORD_FILE="+recoveryPassword)
	run(resticEnv, "restic", "init")
	run(resticEnv, "restic", "backup", "--tag", "control-plane", stage)
	run(resticEnv, "restic", "check", "--read-data")
	// Lose the original database and the in-memory key before recovery.
	source.Close()
	if _, err = admin.ExecContext(ctx, `DROP DATABASE `+dbName+` WITH (FORCE)`); err != nil {
		t.Fatal(err)
	}
	clear(master)
	vault = nil
	restoredFiles := filepath.Join(root, "restored-files")
	run(resticEnv, "restic", "restore", "latest", "--target", restoredFiles)
	var recoveredDump, recoveredKey string
	err = filepath.WalkDir(restoredFiles, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == "database.dump" {
			recoveredDump = path
		}
		if entry.Name() == "vault.key" {
			recoveredKey = path
		}
		return nil
	})
	if err != nil || recoveredDump == "" || recoveredKey == "" {
		t.Fatal("recovery bundle incomplete")
	}
	restored := newDB(dbName + "_new")
	run(pgEnv, "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", dbName+"_new", recoveredDump)
	if err = Quarantine(ctx, restored); err != nil {
		t.Fatal(err)
	}
	if store.Ready(ctx, restored) == nil {
		t.Fatal("quarantined database admitted startup")
	}
	var count int
	if err = restored.QueryRow(`SELECT (SELECT count(*) FROM sessions)+(SELECT count(*) FROM passkeys)+(SELECT count(*) FROM recovery_codes)`).Scan(&count); err != nil || count != 0 {
		t.Fatal("restored credentials could be replayed", err)
	}
	var state string
	if err = restored.QueryRow(`SELECT state FROM deployment_jobs WHERE id='pending'`).Scan(&state); err != nil || state != "uncertain" {
		t.Fatal("pending job would replay", err)
	}
	var stopped, suspended bool
	if err = restored.QueryRow(`SELECT stopped FROM automation_control`).Scan(&stopped); err != nil || !stopped {
		t.Fatal("automation not stopped", err)
	}
	if err = restored.QueryRow(`SELECT suspended FROM backup_policies WHERE id='fixture'`).Scan(&suspended); err != nil || !suspended {
		t.Fatal("backups not suspended", err)
	}
	if err = restored.QueryRow(`SELECT count(*) FROM audit_events WHERE action='secret.created' OR action='control_plane.restored'`).Scan(&count); err != nil || count < 1 {
		t.Fatal("audit not restored", err)
	}
	if _, err = restored.Exec(`DELETE FROM audit_events`); err == nil {
		t.Fatal("audit append-only protection lost")
	}
	vault, err = operations.NewVault(restored, recoveredKey)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := vault.Reveal(ctx, principal, secret.ID, secret.Destination, secret.Revision)
	if err != nil || !bytes.Equal(plain, []byte("private-restore-sentinel")) {
		t.Fatal("vault recovery failed")
	}
	clear(plain)
	wrongKey := filepath.Join(root, "wrong.key")
	if err = os.WriteFile(wrongKey, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	wrong, err := operations.NewVault(restored, wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Reveal(ctx, principal, secret.ID, secret.Destination, secret.Revision); err == nil {
		t.Fatal("wrong recovery key accepted")
	}
	if err = Release(ctx, restored); err != nil {
		t.Fatal(err)
	}
	if err = store.Ready(ctx, restored); err != nil {
		t.Fatal(err)
	}
	if err = restored.QueryRow(`SELECT stopped FROM automation_control`).Scan(&stopped); err != nil || !stopped {
		t.Fatal("release resumed automation")
	}
	t.Logf("control-plane restore drill: database + encrypted vault + audit recovered; replay quarantined; elapsed=%s", time.Since(started).Round(time.Millisecond))
}
