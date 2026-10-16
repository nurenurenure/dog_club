package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/platform/config"
	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/mail"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 4 || os.Args[1] != "bootstrap" {
		return errors.New("usage: admin bootstrap <login> <email>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := database.New(ctx, cfg.DatabaseURL, cfg.DBPingTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	mailer, err := mail.NewLocalSMTP(cfg.SMTPAddr, cfg.MailFrom)
	if err != nil {
		return err
	}
	service, err := auth.NewService(auth.NewRepository(pool), mailer, auth.Options{JWTKey: cfg.JWTKey, Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, IOTimeout: cfg.AuthIOTimeout})
	if err != nil {
		return err
	}
	i, err := service.Bootstrap(ctx, os.Args[2], os.Args[3])
	if err != nil {
		if errors.Is(err, auth.ErrMail) {
			fmt.Fprintln(os.Stderr, "Invitation saved; start Mailpit and repeat the same bootstrap command.")
		}
		return err
	}
	fmt.Fprintf(os.Stdout, "Administrator invitation sent to local Mailpit (invitation %s). Activate it through the API; no password was generated.\n", i.ID)
	return nil
}
