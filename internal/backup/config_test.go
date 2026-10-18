package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func testConfig(t *testing.T) Config {
	root := t.TempDir()
	return Config{Version: 1, WorkRoot: root, CoordinationRoot: filepath.Join(root, "guard"), Repositories: []Repository{{ID: "local", LocationFile: filepath.Join(root, "repository"), PasswordFile: filepath.Join(root, "password"), StorageHost: "source"}}, Policies: []Policy{{ID: "pilot", Project: "pilot", Environment: "test", Server: "source", Repository: "local", Kind: "files", Path: filepath.Join(root, "source"), Enabled: true, Consistency: "crash", TimeoutSeconds: 60, Retention: Retention{KeepLast: 2}}}, Targets: []Target{{ID: "restore", Policy: "pilot", Server: "test", Kind: "files", TestOnly: true, Root: filepath.Join(root, "restores")}}}
}
func TestConfigurationBoundaries(t *testing.T) {
	c := testConfig(t)
	if c.Validate() != nil {
		t.Fatal("valid fixture refused")
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Repositories[0].Offsite = true },
		func(c *Config) { c.Policies[0].Retention.KeepLast = 0 },
		func(c *Config) { c.Policies[0].Kind = "volume" },
		func(c *Config) { c.Policies[0].Path = "../source" },
		func(c *Config) { c.Policies[0].RestoreTestSeconds = 60 },
		func(c *Config) { c.Policies[0].IntervalSeconds = 1 },
		func(c *Config) { c.Targets[0].TestOnly = false },
		func(c *Config) { c.Targets[0].Root = c.Policies[0].Path },
	} {
		c := testConfig(t)
		change(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}
func TestRepositoryRejectsExecutableBackendsAndInheritedCredentials(t *testing.T) {
	c := testConfig(t)
	r := c.Repositories[0]
	if err := os.WriteFile(r.PasswordFile, []byte("isolated-fixture-password"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, loc := range []string{"sftp:host:/repo", "rclone:remote:repo", "rest:http://user:password@example.invalid/repo", "s3:http://example.invalid/repo", "/repo\n--insecure-tls"} {
		if err := os.WriteFile(r.LocationFile, []byte(loc), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := repositoryEnv(r); err == nil {
			t.Fatal("unsafe repository accepted")
		}
	}
	if err := os.WriteFile(r.LocationFile, []byte(filepath.Join(t.TempDir(), "repo")), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RESTIC_PASSWORD", "must-not-inherit")
	t.Setenv("PGPASSWORD", "must-not-inherit")
	env, err := repositoryEnv(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range env {
		if v == "RESTIC_PASSWORD=must-not-inherit" || v == "PGPASSWORD=must-not-inherit" {
			t.Fatal("ambient credentials inherited")
		}
	}
}
func TestPathBoundary(t *testing.T) {
	root := t.TempDir()
	if !within(root, filepath.Join(root, "child")) || within(root, root+"-sibling") {
		t.Fatal("path boundary")
	}
}

func TestLongProgressPreservesFinalSummary(t *testing.T) {
	b := &tail{limit: 64 * 1024}
	for i := 0; i < 100000; i++ {
		_, _ = b.Write([]byte("{\"message_type\":\"status\",\"percent_done\":0.5}\n"))
	}
	summary := []byte("{\"message_type\":\"summary\",\"snapshot_id\":\"complete\"}\n")
	_, _ = b.Write(summary)
	if len(b.data) > b.limit || !bytes.HasSuffix(b.data, summary) {
		t.Fatal("long progress lost bounded final evidence")
	}
}
