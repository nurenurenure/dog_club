package config

import (
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"DATABASE_URL": "postgres://localhost/dogclub"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" || cfg.LogLevel != slog.LevelInfo ||
		cfg.DBPingTimeout != time.Second || cfg.ShutdownTimeout != 10*time.Second ||
		cfg.ReadHeaderTimeout != 5*time.Second || cfg.ReadTimeout != 15*time.Second ||
		cfg.WriteTimeout != 15*time.Second || cfg.IdleTimeout != time.Minute ||
		!cfg.CookieSecure || len(cfg.JWTKey) != 32 || cfg.AuthIOTimeout != 10*time.Second ||
		cfg.SMTPAddr != "127.0.0.1:1025" {
		t.Fatalf("unexpected defaults: HTTP=%s, level=%s, ping=%s, shutdown=%s",
			cfg.HTTPAddr, cfg.LogLevel, cfg.DBPingTimeout, cfg.ShutdownTimeout)
	}
}

func TestInsecureCookieRequiresLoopbackHTTP(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8080", ":8080", "[::]:8080", "localhost:8080"} {
		_, err := load(env(map[string]string{
			"DATABASE_URL": "postgres://localhost/dogclub", "HTTP_ADDR": address,
			"AUTH_COOKIE_SECURE": "false",
		}))
		if err == nil || !strings.Contains(err.Error(), "AUTH_COOKIE_SECURE") {
			t.Fatal("insecure cookie was allowed without an explicit loopback IP")
		}
	}
	for _, address := range []string{"127.0.0.1:8080", "[::1]:8080"} {
		if _, err := load(env(map[string]string{
			"DATABASE_URL": "postgres://localhost/dogclub", "HTTP_ADDR": address,
			"AUTH_COOKIE_SECURE": "false",
		})); err != nil {
			t.Fatal("insecure cookie was rejected for local development")
		}
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL": "postgres://localhost/dogclub", "HTTP_ADDR": "[::1]:9090",
		"LOG_LEVEL": "debug", "DB_PING_TIMEOUT": "250ms", "SHUTDOWN_TIMEOUT": "3s",
		"HTTP_READ_HEADER_TIMEOUT": "2s", "HTTP_READ_TIMEOUT": "4s",
		"HTTP_WRITE_TIMEOUT": "5s", "HTTP_IDLE_TIMEOUT": "30s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "[::1]:9090" || cfg.LogLevel != slog.LevelDebug ||
		cfg.DBPingTimeout != 250*time.Millisecond || cfg.ShutdownTimeout != 3*time.Second ||
		cfg.ReadHeaderTimeout != 2*time.Second || cfg.ReadTimeout != 4*time.Second ||
		cfg.WriteTimeout != 5*time.Second || cfg.IdleTimeout != 30*time.Second {
		t.Fatal("environment overrides were not applied")
	}
}

func TestLoadRejectsInvalidConfigWithoutLeakingValues(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"missing database", "DATABASE_URL", ""},
		{"blank database", "DATABASE_URL", "  "},
		{"missing HTTP port", "HTTP_ADDR", "127.0.0.1"},
		{"invalid HTTP port", "HTTP_ADDR", "127.0.0.1:secret_value"},
		{"zero HTTP port", "HTTP_ADDR", "127.0.0.1:0"},
		{"large HTTP port", "HTTP_ADDR", "127.0.0.1:65536"},
		{"bad level", "LOG_LEVEL", "secret_value"},
		{"empty level", "LOG_LEVEL", ""},
		{"bad ping timeout", "DB_PING_TIMEOUT", "secret_value"},
		{"zero ping timeout", "DB_PING_TIMEOUT", "0s"},
		{"negative shutdown", "SHUTDOWN_TIMEOUT", "-1s"},
		{"missing JWT key", "AUTH_JWT_KEY", ""},
		{"invalid JWT key", "AUTH_JWT_KEY", "secret_value"},
		{"short JWT key", "AUTH_JWT_KEY", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"empty issuer", "AUTH_JWT_ISSUER", ""},
		{"empty audience", "AUTH_JWT_AUDIENCE", ""},
		{"invalid cookie flag", "AUTH_COOKIE_SECURE", "secret_value"},
		{"external SMTP", "SMTP_ADDR", "203.0.113.1:25"},
		{"bad SMTP port", "SMTP_ADDR", "127.0.0.1:secret_value"},
		{"bad auth timeout", "AUTH_IO_TIMEOUT", "0s"},
	}
	for _, key := range []string{"HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT"} {
		tests = append(tests, struct{ name, key, value string }{key, key, "0s"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(env(map[string]string{"DATABASE_URL": "postgres://localhost/dogclub", tt.key: tt.value}))
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("expected error naming %s, got %v", tt.key, err)
			}
			if strings.Contains(err.Error(), "secret_value") {
				t.Fatal("configuration error leaked an input value")
			}
		})
	}
}

func env(values map[string]string) func(string) (string, bool) {
	if _, exists := values["AUTH_JWT_KEY"]; !exists {
		values["AUTH_JWT_KEY"] = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	}
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
