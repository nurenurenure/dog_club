package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/breeding"
	"dogclub.local/backend/internal/documents"
	"dogclub.local/backend/internal/dogs"
	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/health"
	"dogclub.local/backend/internal/members"
	"dogclub.local/backend/internal/notifications"
	"dogclub.local/backend/internal/operations"
	"dogclub.local/backend/internal/platform/config"
	"dogclub.local/backend/internal/platform/database"
	"dogclub.local/backend/internal/platform/filestore"
	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/mail"
	"dogclub.local/backend/internal/platform/middleware"
	"dogclub.local/backend/internal/platform/notify"
	"dogclub.local/backend/internal/reports"
	"dogclub.local/backend/internal/training"
	"dogclub.local/backend/internal/veterinary"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type App struct {
	cfg       config.Config
	logger    *slog.Logger
	server    *http.Server
	health    *health.Handler
	worker    func(context.Context, *slog.Logger)
	closePool func()
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	pool, err := database.New(context.Background(), cfg.DatabaseURL, cfg.DBPingTimeout)
	if err != nil {
		return nil, err
	}
	mailer, err := mail.NewLocalSMTP(cfg.SMTPAddr, cfg.MailFrom, cfg.LocalCompose)
	if err != nil {
		pool.Close()
		return nil, err
	}
	service, err := auth.NewService(auth.NewRepository(pool), mailer, auth.Options{JWTKey: cfg.JWTKey, Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, IOTimeout: cfg.AuthIOTimeout})
	if err != nil {
		pool.Close()
		return nil, err
	}
	authHandler := auth.NewHandler(service, cfg.CookieSecure)
	memberService := members.NewService(service)
	dogService := dogs.NewService(service, memberService, veterinary.DogDatesPolicy{})
	vetService := veterinary.NewService(service, dogService, memberService)
	dogService.SetBreedingPolicy(breeding.DogPolicy{})
	breedingService := breeding.NewService(service, dogService, vetService)
	dogService.SetEventPolicy(events.DogPolicy{})
	eventService := events.NewService(service, memberService, dogService, vetService)
	trainingService := training.NewService(service, memberService, dogService, vetService, eventService)
	dogService.SetTrainingPolicy(training.DogPolicy{})
	files, err := filestore.New(cfg.DocumentsDir)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("private document storage unavailable")
	}
	dogService.SetPhotoStore(files)
	reportService := reports.NewService(service, memberService, dogService, breedingService, eventService)
	documentService := documents.NewService(service, memberService, dogService, vetService, files, breedingService, eventService)
	notifier := notifications.New(pool, service, memberService, dogService, mailer, map[string]notify.Provider{"dogs": dogService, "veterinary": vetService, "breeding": breedingService, "events": eventService, "documents": documentService})
	dogService.SetNotificationSink(notifier)
	breedingService.SetNotificationSink(notifier)
	eventService.SetNotificationSink(notifier)
	documentService.SetNotificationSink(notifier)
	metrics := &middleware.HTTPMetrics{}
	application := newAppWithMetrics(metrics, cfg, logger, pool.Ping, func() { _ = files.Close(); pool.Close() }, authHandler,
		members.NewHandler(memberService, authHandler), dogs.NewHandler(dogService, authHandler), veterinary.NewHandler(vetService, authHandler), breeding.NewHandler(breedingService, authHandler), events.NewHandler(eventService, authHandler), documents.NewHandler(documentService, authHandler), reports.NewHandler(reportService, authHandler), training.NewHandler(trainingService, authHandler), operations.NewHandler(service, authHandler, metrics, pool, notifier, cfg.NotificationsEnabled))
	if cfg.NotificationsEnabled {
		application.worker = notifier.Run
	}
	return application, nil
}

func newApp(cfg config.Config, logger *slog.Logger, ping func(context.Context) error, closePool func(), apiHandlers ...interface{ RegisterRoutes(chi.Router) }) *App {
	return newAppWithMetrics(nil, cfg, logger, ping, closePool, apiHandlers...)
}

func newAppWithMetrics(metrics *middleware.HTTPMetrics, cfg config.Config, logger *slog.Logger, ping func(context.Context) error, closePool func(), apiHandlers ...interface{ RegisterRoutes(chi.Router) }) *App {
	h := health.New(ping, cfg.DBPingTimeout)
	router := chi.NewRouter()
	if metrics != nil {
		router.Use(metrics.Observe)
	}
	router.Use(chimiddleware.RequestID)
	router.Use(middleware.RequestLog(logger))
	router.Use(middleware.Recover(logger))
	router.NotFound(httpapi.NotFound)
	router.MethodNotAllowed(httpapi.MethodNotAllowed(router))
	router.Get("/health/live", h.Live)
	router.Get("/health/ready", h.Ready)
	if len(apiHandlers) > 0 {
		router.Route("/api/v1", func(r chi.Router) {
			for _, h := range apiHandlers {
				h.RegisterRoutes(r)
			}
		})
	}
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
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	if a.worker != nil {
		go func() { defer close(workerDone); a.worker(workerCtx, a.logger) }()
	} else {
		close(workerDone)
	}
	defer func() { stopWorker(); <-workerDone }()
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
