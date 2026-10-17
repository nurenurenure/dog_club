package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	LogLevel          slog.Level
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	DBPingTimeout     time.Duration
	ShutdownTimeout   time.Duration
	JWTKey            []byte
	JWTIssuer         string
	JWTAudience       string
	AuthIOTimeout     time.Duration
	CookieSecure      bool
	SMTPAddr          string
	DocumentsDir      string
	MailFrom          string
}

// Load reads only the process environment; it never loads .env files.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookup func(string) (string, bool)) (Config, error) {
	value := func(key, fallback string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return fallback
	}
	cfg := Config{
		HTTPAddr:    value("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL: value("DATABASE_URL", ""),
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	key, err := base64.StdEncoding.DecodeString(value("AUTH_JWT_KEY", ""))
	if err != nil || len(key) < 32 {
		return Config{}, fmt.Errorf("AUTH_JWT_KEY must be base64 containing at least 32 random bytes")
	}
	cfg.JWTKey = key
	cfg.JWTIssuer = value("AUTH_JWT_ISSUER", "dogclub.local")
	cfg.JWTAudience = value("AUTH_JWT_AUDIENCE", "dogclub-api")
	if cfg.JWTIssuer == "" || cfg.JWTAudience == "" {
		return Config{}, fmt.Errorf("AUTH_JWT_ISSUER and AUTH_JWT_AUDIENCE must not be empty")
	}
	cfg.CookieSecure, err = strconv.ParseBool(value("AUTH_COOKIE_SECURE", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE must be a boolean")
	}
	cfg.SMTPAddr = value("SMTP_ADDR", "127.0.0.1:1025")
	smtpHost, smtpPort, smtpErr := net.SplitHostPort(cfg.SMTPAddr)
	smtpNumber, smtpNumberErr := strconv.Atoi(smtpPort)
	if smtpErr != nil || smtpNumberErr != nil || smtpNumber < 1 || smtpNumber > 65535 || !net.ParseIP(smtpHost).IsLoopback() {
		return Config{}, fmt.Errorf("SMTP_ADDR must use a loopback IP and a valid port")
	}
	cfg.DocumentsDir = value("DOCUMENTS_DIR", "var/documents")
	if strings.TrimSpace(cfg.DocumentsDir) == "" {
		return Config{}, fmt.Errorf("DOCUMENTS_DIR must not be empty")
	}
	cfg.MailFrom = value("MAIL_FROM", "noreply@dogclub.test")
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR must be host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR must contain a port from 1 to 65535")
	}
	host, _, _ := net.SplitHostPort(cfg.HTTPAddr)
	if !cfg.CookieSecure && !net.ParseIP(host).IsLoopback() {
		return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE=false requires a loopback HTTP_ADDR")
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL must be a valid slog level")
	}
	for _, setting := range []struct {
		key      string
		fallback string
		target   *time.Duration
	}{
		{"HTTP_READ_HEADER_TIMEOUT", "5s", &cfg.ReadHeaderTimeout},
		{"HTTP_READ_TIMEOUT", "15s", &cfg.ReadTimeout},
		{"HTTP_WRITE_TIMEOUT", "15s", &cfg.WriteTimeout},
		{"HTTP_IDLE_TIMEOUT", "60s", &cfg.IdleTimeout},
		{"DB_PING_TIMEOUT", "1s", &cfg.DBPingTimeout},
		{"SHUTDOWN_TIMEOUT", "10s", &cfg.ShutdownTimeout},
		{"AUTH_IO_TIMEOUT", "10s", &cfg.AuthIOTimeout},
	} {
		duration, err := time.ParseDuration(value(setting.key, setting.fallback))
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", setting.key)
		}
		*setting.target = duration
	}
	return cfg, nil
}
