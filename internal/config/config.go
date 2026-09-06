package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

func Secret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(b))
	if len(value) < 32 || len(value) > 256 {
		return "", errors.New("secret must contain 32 to 256 characters")
	}
	return value, nil
}
func DatabaseURL() (string, error) {
	if value := os.Getenv("PRISME_DATABASE_URL"); value != "" {
		return value, nil
	}
	password, err := Secret(os.Getenv("PRISME_DB_PASSWORD_FILE"))
	if err != nil {
		return "", err
	}
	host := os.Getenv("PRISME_DB_HOST")
	if host == "" {
		host = "localhost:5432"
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword("prisme", password), Host: host, Path: "prisme"}
	u.RawQuery = "sslmode=disable&connect_timeout=5" // isolated local Compose network only
	return u.String(), nil
}
