package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if _, err := FromEnv(env(nil)); err != nil {
		t.Fatalf("empty environment must load: %v", err)
	}
}

func TestPublicListenRequiresOptIn(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "203.0.113.10:8080", "[::]:8080"} {
		if _, err := FromEnv(env(map[string]string{"DP_LISTEN": addr})); err == nil {
			t.Errorf("%s should be rejected without DP_ALLOW_PUBLIC_LISTEN", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:9000", "localhost:9000", "[::1]:9000"} {
		if _, err := FromEnv(env(map[string]string{"DP_LISTEN": addr})); err != nil {
			t.Errorf("%s should be accepted: %v", addr, err)
		}
	}
	c, err := FromEnv(env(map[string]string{"DP_LISTEN": "0.0.0.0:8080", "DP_ALLOW_PUBLIC_LISTEN": "true"}))
	if err != nil || c.Listen != "0.0.0.0:8080" {
		t.Fatalf("opt-in should allow a public listener: %v", err)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"bad log level":      {"DP_LOG_LEVEL": "loud"},
		"postgres not ready": {"DP_DB_DRIVER": "postgres"},
		"unknown driver":     {"DP_DB_DRIVER": "mysql"},
		"bad bool":           {"DP_ALLOW_PUBLIC_LISTEN": "maybe"},
		"bad module":         {"DP_MODULES": "dns,Bad Name"},
		"bad ttl":            {"DP_LOG_TTL": "soon"},
		"zero ttl":           {"DP_METRIC_TTL": "0s"},
		"no port":            {"DP_LISTEN": "127.0.0.1"},
	}
	for name, vars := range cases {
		if _, err := FromEnv(env(vars)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestModulesAndTTLs(t *testing.T) {
	c, err := FromEnv(env(map[string]string{
		"DP_MODULES":    " WordPress, dns ,,waf ",
		"DP_LOG_TTL":    "14d",
		"DP_METRIC_TTL": "36h",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Modules, ","); got != "wordpress,dns,waf" {
		t.Errorf("modules = %q", got)
	}
	if c.LogTTL != 14*24*time.Hour || c.MetricTTL != 36*time.Hour {
		t.Errorf("ttls = %v, %v", c.LogTTL, c.MetricTTL)
	}
}

func TestCheckSecretKey(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "secret.key")
	if err := os.WriteFile(key, []byte(strings.Repeat("a", 64)), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Default()
	c.SecretKeyFile = key
	if err := c.CheckSecretKey(); err == nil {
		t.Error("mode 0644 must be rejected")
	}
	if err := os.Chmod(key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.CheckSecretKey(); err != nil {
		t.Errorf("mode 0600 should pass: %v", err)
	}
	short := filepath.Join(dir, "short.key")
	if err := os.WriteFile(short, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.SecretKeyFile = short
	if err := c.CheckSecretKey(); err == nil {
		t.Error("a short key must be rejected")
	}
	c.SecretKeyFile = filepath.Join(dir, "missing.key")
	if err := c.CheckSecretKey(); err == nil {
		t.Error("a missing key file must be rejected")
	}
	c.SecretKeyFile = ""
	if err := c.CheckSecretKey(); err != nil {
		t.Errorf("no key file configured should be a no-op: %v", err)
	}
}
