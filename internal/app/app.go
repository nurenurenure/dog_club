package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"dogclub.local/backend/internal/health"
	"dogclub.local/backend/internal/platform/config"
	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type App struct {
	cfg       config.Config
	logger    *slog.Logger
	server    *http.Server
	health    *health.Handler
	closePool func()
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	pool, err := database.New(context.Background(), cfg.DatabaseURL, cfg.DBPingTimeout)
	if err != nil {
		return nil, err
	}
	return newApp(cfg, logger, pool.Ping, pool.Close), nil
}

func newApp(cfg config.Config, logger *slog.Logger, ping func(context.Context) error, closePool func()) *App {
	h := health.New(ping, cfg.DBPingTimeout)
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(middleware.RequestLog(logger))
	router.Use(middleware.Recover(logger))
	router.Get("/health/live", h.Live)
	router.Get("/health/ready", h.Ready)
	return &App{
		cfg: cfg, logger: logger, health: h, closePool: closePool,
		server: &http.Server{
			Addr: cfg.HTTPAddr, Handler: router,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
	}
}

// Run owns the listener and pool. An App is run once.
func (a *App) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.cfg.HTTPAddr)
	if err != nil {
		a.closePool()
		return fmt.Errorf("listen HTTP: %w", err)
	}
	return a.serve(ctx, listener)
}

func (a *App) serve(ctx context.Context, listener net.Listener) error {
	defer a.closePool()
	// Shutdown must let in-flight requests finish before canceling their context.
	requestContext, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	a.server.BaseContext = func(net.Listener) context.Context { return requestContext }
	served := make(chan error, 1)
	go func() { served <- a.server.Serve(listener) }()
	a.logger.Info("http server started", "address", listener.Addr().String())
	select {
	case err := <-served:
		a.health.Stop()
		_ = a.server.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		a.health.Stop()
		a.logger.Info("http server stopping")
		shutdownContext, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownContext); err != nil {
			cancelRequests()
			_ = a.server.Close()
			return fmt.Errorf("shutdown HTTP: %w", err)
		}
		a.logger.Info("http server stopped")
		return nil
	}
}
