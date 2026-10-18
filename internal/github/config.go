// Package github owns the private GitHub App integration. Provider data is untrusted.
package github

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Repository struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
	WorkflowID  string `json:"workflow_id"`
}
type Config struct {
	AppID             string       `json:"app_id"`
	InstallationID    string       `json:"installation_id"`
	PrivateKeyFile    string       `json:"private_key_file"`
	WebhookSecretFile string       `json:"webhook_secret_file"`
	Repositories      []Repository `json:"repositories"`
}

var Number = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
var SHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var environmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$`)

func Load(path string) (Config, *rsa.PrivateKey, []byte, error) {
	var c Config
	fail := errors.New("invalid GitHub App configuration")
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 65536 {
		return c, nil, nil, fail
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || !Number.MatchString(c.AppID) || !Number.MatchString(c.InstallationID) || len(c.Repositories) == 0 || len(c.Repositories) > 20 {
		return c, nil, nil, fail
	}
	seen := map[string]bool{}
	for _, r := range c.Repositories {
		if !Number.MatchString(r.ID) || !Number.MatchString(r.WorkflowID) || !namePattern.MatchString(r.Name) || len(r.Name) > 200 || !environmentPattern.MatchString(r.Environment) || seen[r.ID] || seen[strings.ToLower(r.Name)] {
			return c, nil, nil, fail
		}
		seen[r.ID] = true
		seen[strings.ToLower(r.Name)] = true
	}
	pem, err := os.ReadFile(c.PrivateKeyFile)
	if err != nil {
		return c, nil, nil, fail
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pem)
	if err != nil || key.N.BitLen() < 2048 {
		return c, nil, nil, fail
	}
	secret, err := os.ReadFile(c.WebhookSecretFile)
	secret = []byte(strings.TrimSpace(string(secret)))
	if err != nil || len(secret) < 32 || len(secret) > 256 {
		return c, nil, nil, fail
	}
	return c, key, secret, nil
}
