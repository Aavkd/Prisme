package api_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prisme.local/platform/internal/backup"
	"prisme.local/platform/internal/store"
)

func TestPhaseSevenLiveResticPostgresAndFiles(t *testing.T) {
	if os.Getenv("PRISME_TEST_BACKUP") != "1" {
		t.Skip("set PRISME_TEST_BACKUP=1 with Restic and PostgreSQL CLI installed")
	}
	for _, bin := range []string{"restic", "pg_dump", "pg_restore", "psql", "createdb", "dropdb"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Fatal("required fixture binary unavailable: " + bin)
		}
	}
	f, _, e, _ := backupFixture(t)
	ctx := context.Background()
	// Only this random fixture database and role are created/dropped. No pilot
	// production data is read. Logical content resembles a small worker project.
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	source := "prisme_pilot_" + suffix
	role := "prisme_test_" + suffix
	if _, err := f.db.Exec(`CREATE ROLE ` + role + ` LOGIN CREATEDB`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.db.Exec(`DROP ROLE ` + role) })
	if _, err := f.db.Exec(`CREATE DATABASE ` + source + ` TEMPLATE template0`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.db.Exec(`DROP DATABASE ` + source + ` WITH (FORCE)`) })
	u, err := url.Parse(os.Getenv("PRISME_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + source
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	sourceDB, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer sourceDB.Close()
	if _, err = sourceDB.Exec(`CREATE TABLE jobs(id integer PRIMARY KEY,state text NOT NULL,payload text NOT NULL); INSERT INTO jobs VALUES(1,'queued','alpha'),(2,'complete','beta'); CREATE INDEX jobs_state ON jobs(state)`); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	restoreRoot := filepath.Join(root, "restores")
	files := filepath.Join(root, "files")
	for _, d := range []string{work, restoreRoot, files} {
		if err = os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	host := u.Hostname()
	if h := u.Query().Get("host"); h != "" {
		host = h
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	user := "postgres"
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	base := "host=" + host + "\nport=" + port + "\nsslmode=disable\n"
	service := write("pg-service.conf", "[source]\n"+base+"dbname="+source+"\nuser="+user+"\npassword="+password+"\n[target]\n"+base+"dbname=postgres\nuser="+role+"\n")
	validation := write("validate.sql", "SELECT count(*)=2 AND string_agg(payload,',' ORDER BY id)='alpha,beta' AND (SELECT count(*) FROM pg_indexes WHERE tablename='jobs')=2 FROM jobs;")
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	r := backup.Repository{ID: "live", LocationFile: write("repo-location", filepath.Join(root, "encrypted-repository")), PasswordFile: write("repo-password", hex.EncodeToString(b)), StorageHost: "fixture-host"}
	cmd := exec.Command("restic", "--repository-file", r.LocationFile, "--password-file", r.PasswordFile, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Restic init: %s", out)
	}
	p := backup.Policy{ID: "live-db", Project: "sounding-lines-fixture", Environment: "test", Server: "fixture-host", Repository: r.ID, Kind: "postgres", ServiceFile: service, Service: "source", Consistency: "logical", Enabled: true, TimeoutSeconds: 120, Retention: backup.Retention{KeepLast: 2}}
	filePolicy := backup.Policy{ID: "live-files", Project: "pilot-files-fixture", Environment: "test", Server: "fixture-host", Repository: r.ID, Kind: "volume", Path: files, Consistency: "stopped", Enabled: true, TimeoutSeconds: 120, Retention: backup.Retention{KeepLast: 2}}
	target := backup.Target{ID: "live-test", Policy: p.ID, Server: "fixture-host", Kind: "postgres", TestOnly: true, ServiceFile: service, Service: "target", ValidationFile: validation}
	e.Config = backup.Config{Version: 1, WorkRoot: work, CoordinationRoot: filepath.Join(root, "guard"), Repositories: []backup.Repository{r}, Policies: []backup.Policy{p, filePolicy}, Targets: []backup.Target{target, {ID: "live-file-test", Policy: filePolicy.ID, Server: "fixture-host", Kind: "files", TestOnly: true, Root: restoreRoot}}}
	e.Driver = &backup.Restic{}
	if err = e.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	run := func(req backup.Request) backup.Job {
		t.Helper()
		j, err := e.Prepare(ctx, req, "live-"+fmt.Sprintf("%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		if j.State == "planned" {
			j, err = e.Decide(ctx, j.ID, "approve", backup.Confirmation(j), j.Revision, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
		}
		if err = e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		j, err = e.Job(ctx, j.ID)
		if err != nil || j.State != "succeeded" {
			t.Fatalf("%s: state=%s step=%s code=%s evidence=%v", req.Kind, j.State, j.Step, j.ErrorCode, j.Evidence)
		}
		return j
	}
	run(backup.Request{Policy: p.ID, Kind: "backup"})
	as, err := e.Artifacts(ctx, 1)
	if err != nil || len(as) != 1 {
		t.Fatal("verified DB artifact absent")
	}
	dbArtifact := as[0]
	if dbArtifact.Bytes == 0 {
		t.Fatal("empty dump")
	}
	j := run(backup.Request{Policy: p.ID, Kind: "restore", Artifact: dbArtifact.ID, Target: target.ID})
	database := j.Evidence["database"].(string)
	t.Cleanup(func() { _, _ = f.db.Exec(`DROP DATABASE ` + database + ` WITH (FORCE)`) })
	if j.Evidence["validation"] != "passed" {
		t.Fatal("functional DB validation absent")
	}
	// Scheduled drills must validate and remove only their own fresh database.
	drill, err := e.Prepare(ctx, backup.Request{Policy: p.ID, Kind: "restore", Artifact: dbArtifact.ID, Target: target.ID}, "scheduled-drill-12345678")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Decide(ctx, drill.ID, "approve", backup.Confirmation(drill), drill.Revision, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE backup_jobs SET scheduled=true WHERE id=$1`, drill.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	drill, _ = e.Job(ctx, drill.ID)
	if drill.State != "succeeded" || drill.Evidence["cleanup"] != "passed" {
		t.Fatalf("scheduled restore cleanup: %+v", drill)
	}
	var remains bool
	if err = f.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, drill.Evidence["database"]).Scan(&remains); err != nil || remains {
		t.Fatal("scheduled test database remained")
	}
	// Failed validation leaves a failed restore and never advertises success.
	if err = os.WriteFile(validation, []byte("SELECT false;"), 0600); err != nil {
		t.Fatal(err)
	}
	failed, err := e.Prepare(ctx, backup.Request{Policy: p.ID, Kind: "restore", Artifact: dbArtifact.ID, Target: target.ID}, "failed-validation-123456")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Decide(ctx, failed.ID, "approve", backup.Confirmation(failed), failed.Revision, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	failed, _ = e.Job(ctx, failed.ID)
	if failed.State != "failed" || failed.Step != "validate_database" {
		t.Fatal("invalid data accepted")
	}
	if database, ok := failed.Evidence["database"].(string); ok {
		t.Cleanup(func() { _, _ = f.db.Exec(`DROP DATABASE ` + database + ` WITH (FORCE)`) })
	}
	if err = os.WriteFile(filepath.Join(files, "record.txt"), []byte("fixture-volume-content"), 0600); err != nil {
		t.Fatal(err)
	}
	run(backup.Request{Policy: filePolicy.ID, Kind: "backup"})
	as, _ = e.Artifacts(ctx, 1)
	var fileArtifact backup.Artifact
	for _, a := range as {
		if a.PolicyID == filePolicy.ID {
			fileArtifact = a
			break
		}
	}
	j = run(backup.Request{Policy: filePolicy.ID, Kind: "restore", Artifact: fileArtifact.ID, Target: "live-file-test"})
	found := false
	err = filepath.WalkDir(j.Evidence["output_directory"].(string), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "record.txt" {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			found = string(b) == "fixture-volume-content"
		}
		return nil
	})
	if err != nil || !found {
		t.Fatal("volume content not restored")
	}
	// Retention is scoped to the policy even when policies share a repository.
	for i := 0; i < 2; i++ {
		time.Sleep(time.Second)
		run(backup.Request{Policy: p.ID, Kind: "backup"})
	}
	// A snapshot left in Restic without a durable successful job cannot evict
	// the newest verified copies when keep-last is evaluated.
	for i := 0; i < 2; i++ {
		probe := exec.Command("restic", "--repository-file", r.LocationFile, "--password-file", r.PasswordFile, "backup", "--host", p.Server, "--tag", "prisme:"+p.ID, "--stdin-from-command", "--stdin-filename", "database.dump", "--", "pg_dump", "--format=custom", "--no-owner", "--no-acl", "--no-password")
		probe.Env = append(os.Environ(), "PGSERVICEFILE="+service, "PGSERVICE=source")
		if probe.Run() != nil {
			t.Fatal("unregistered fixture snapshot failed")
		}
	}
	run(backup.Request{Policy: p.ID, Kind: "retention"})
	as, _ = e.Artifacts(ctx, 1)
	expired := false
	for _, a := range as {
		if a.ID == dbArtifact.ID {
			expired = a.ExpiredAt != nil
		}
		if a.ID == fileArtifact.ID && a.ExpiredAt != nil {
			t.Fatal("other policy pruned")
		}
	}
	if !expired {
		t.Fatal("retention did not expire oldest snapshot")
	}
	// Wrong encryption key fails a full check and raises an incident.
	if err = os.WriteFile(r.PasswordFile, []byte(strings.Repeat("wrong-key", 8)), 0600); err != nil {
		t.Fatal(err)
	}
	bad, err := e.Prepare(ctx, backup.Request{Policy: filePolicy.ID, Kind: "check"}, "wrong-password-test-12345")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	bad, _ = e.Job(ctx, bad.ID)
	if bad.State != "failed" {
		t.Fatal("wrong encryption password accepted")
	}
	if err = os.WriteFile(r.PasswordFile, []byte(hex.EncodeToString(b)), 0600); err != nil {
		t.Fatal(err)
	}
	corrupted := false
	err = filepath.WalkDir(filepath.Join(root, "encrypted-repository", "data"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || corrupted {
			return nil
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		v := []byte{0}
		if _, err = f.ReadAt(v, 0); err != nil {
			return err
		}
		v[0] ^= 255
		_, err = f.WriteAt(v, 0)
		corrupted = err == nil
		return err
	})
	if err != nil || !corrupted {
		t.Fatal("corruption fixture failed")
	}
	corrupt, err := e.Prepare(ctx, backup.Request{Policy: p.ID, Kind: "check"}, "corrupt-repository-123456")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	corrupt, _ = e.Job(ctx, corrupt.ID)
	if corrupt.State != "failed" {
		t.Fatal("damaged encrypted data accepted")
	}
}
