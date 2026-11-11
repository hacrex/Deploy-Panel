// Package config loads dpd settings from environment variables (prefix DP_)
// and validates them. A config file format is planned (see docs/CONFIG.md);
// until then the environment is the single source.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen            string // admin UI and API, loopback by default
	AllowPublicListen bool   // must be true to bind a non-loopback Listen address
	PublicURL         string // external URL of the panel, used for links and ACME
	AgentListen       string // where node agents connect (mTLS)
	DBDriver          string // sqlite (postgres is planned)
	DBPath            string
	SecretKeyFile     string // master key file, must be mode 0600 or stricter
	Modules           []string
	LogLevel          string
	LogTTL            time.Duration
	MetricTTL         time.Duration

	// Session cookie
	SessionCookieName string        // default "dp_session"
	SessionLifetime   time.Duration // default 24h
	SessionInsecure   bool          // allow non-Secure/SameSite=Lax cookies on loopback (dev only)

	// Login rate limiting: max failed attempts per LoginLockoutFor window per (email, ip).
	LoginMaxAttempts int
	LoginLockoutFor  time.Duration
}

func Default() Config {
	return Config{
		Listen:      "127.0.0.1:8080",
		AgentListen: "127.0.0.1:8443",
		DBDriver:    "sqlite",
		DBPath:      "/var/lib/deploypanel/dpd.db",
		LogLevel:    "info",
		LogTTL:      7 * 24 * time.Hour,
		MetricTTL:   30 * 24 * time.Hour,

		SessionCookieName: "dp_session",
		SessionLifetime:   24 * time.Hour,
		SessionInsecure:   false,

		LoginMaxAttempts: 5,
		LoginLockoutFor:  15 * time.Minute,
	}
}

var moduleName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// FromEnv builds a Config from defaults plus DP_* variables, then validates it.
// getenv is os.Getenv in production and a map lookup in tests.
func FromEnv(getenv func(string) string) (Config, error) {
	c := Default()
	var errs []error
	str := func(key string, dst *string) {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	str("DP_LISTEN", &c.Listen)
	str("DP_PUBLIC_URL", &c.PublicURL)
	str("DP_AGENT_LISTEN", &c.AgentListen)
	str("DP_DB_DRIVER", &c.DBDriver)
	str("DP_DB_PATH", &c.DBPath)
	str("DP_SECRET_KEY_FILE", &c.SecretKeyFile)
	str("DP_LOG_LEVEL", &c.LogLevel)
	str("DP_SESSION_COOKIE_NAME", &c.SessionCookieName)
	if v := getenv("DP_SESSION_LIFETIME"); v != "" {
		d, err := ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DP_SESSION_LIFETIME: %w", err))
		} else {
			c.SessionLifetime = d
		}
	}
	if v := getenv("DP_SESSION_INSECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DP_SESSION_INSECURE: %w", err))
		}
		c.SessionInsecure = b
	}
	if v := getenv("DP_LOGIN_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DP_LOGIN_MAX_ATTEMPTS: %w", err))
		} else if n < 1 {
			errs = append(errs, errors.New("DP_LOGIN_MAX_ATTEMPTS must be at least 1"))
		} else {
			c.LoginMaxAttempts = n
		}
	}
	if v := getenv("DP_LOGIN_LOCKOUT_FOR"); v != "" {
		d, err := ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DP_LOGIN_LOCKOUT_FOR: %w", err))
		} else {
			c.LoginLockoutFor = d
		}
	}

	if v := getenv("DP_ALLOW_PUBLIC_LISTEN"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DP_ALLOW_PUBLIC_LISTEN: %w", err))
		}
		c.AllowPublicListen = b
	}
	if v := getenv("DP_MODULES"); v != "" {
		c.Modules = nil
		for _, m := range strings.Split(v, ",") {
			if m = strings.ToLower(strings.TrimSpace(m)); m != "" {
				c.Modules = append(c.Modules, m)
			}
		}
	}
	for key, dst := range map[string]*time.Duration{"DP_LOG_TTL": &c.LogTTL, "DP_METRIC_TTL": &c.MetricTTL} {
		if v := getenv(key); v != "" {
			d, err := ParseDuration(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", key, err))
				continue
			}
			*dst = d
		}
	}
	if err := errors.Join(errs...); err != nil {
		return c, err
	}
	return c, c.Validate()
}

// Validate enforces the secure defaults: the admin listener stays on loopback
// unless the operator opts in explicitly.
func (c Config) Validate() error {
	var errs []error
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		errs = append(errs, fmt.Errorf("DP_LISTEN %q: %w", c.Listen, err))
	} else if !IsLoopbackAddr(c.Listen) && !c.AllowPublicListen {
		errs = append(errs, fmt.Errorf("DP_LISTEN %q is not a loopback address; expose the panel through the proxy "+
			"or set DP_ALLOW_PUBLIC_LISTEN=true to bind it publicly", c.Listen))
	}
	if c.AgentListen != "" {
		if _, _, err := net.SplitHostPort(c.AgentListen); err != nil {
			errs = append(errs, fmt.Errorf("DP_AGENT_LISTEN %q: %w", c.AgentListen, err))
		}
	}
	switch c.DBDriver {
	case "sqlite":
		if c.DBPath == "" {
			errs = append(errs, errors.New("DP_DB_PATH is required for the sqlite driver"))
		}
	case "postgres":
		errs = append(errs, errors.New("DP_DB_DRIVER=postgres is planned but not supported yet"))
	default:
		errs = append(errs, fmt.Errorf("DP_DB_DRIVER %q is not supported (use sqlite)", c.DBDriver))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("DP_LOG_LEVEL %q must be debug, info, warn or error", c.LogLevel))
	}
	for _, m := range c.Modules {
		if !moduleName.MatchString(m) {
			errs = append(errs, fmt.Errorf("DP_MODULES: bad module name %q", m))
		}
	}
	if c.LogTTL <= 0 {
		errs = append(errs, errors.New("DP_LOG_TTL must be positive"))
	}
	if c.MetricTTL <= 0 {
		errs = append(errs, errors.New("DP_METRIC_TTL must be positive"))
	}
	if c.SessionCookieName == "" {
		errs = append(errs, errors.New("DP_SESSION_COOKIE_NAME must not be empty"))
	}
	if c.SessionLifetime <= 0 {
		errs = append(errs, errors.New("DP_SESSION_LIFETIME must be positive"))
	}
	if c.LoginMaxAttempts < 1 {
		errs = append(errs, errors.New("DP_LOGIN_MAX_ATTEMPTS must be at least 1"))
	}
	if c.LoginLockoutFor <= 0 {
		errs = append(errs, errors.New("DP_LOGIN_LOCKOUT_FOR must be positive"))
	}
	return errors.Join(errs...)
}

// IsLoopbackAddr reports whether a host:port address binds only to loopback.
// An empty host (":8080") binds every interface and is not loopback.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckSecretKey verifies the master key file exists, is not group or world
// accessible, and is long enough. It is a no-op when no file is configured.
func (c Config) CheckSecretKey() error {
	if c.SecretKeyFile == "" {
		return nil
	}
	info, err := os.Stat(c.SecretKeyFile)
	if err != nil {
		return fmt.Errorf("secret key file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secret key file %s has mode %o; it must not be accessible by group or others (chmod 600)",
			c.SecretKeyFile, info.Mode().Perm())
	}
	if info.Size() < 32 {
		return fmt.Errorf("secret key file %s is too short (need at least 32 bytes)", c.SecretKeyFile)
	}
	return nil
}

// ParseDuration is time.ParseDuration plus a "d" suffix for whole days ("7d").
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("bad day count %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
