package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dogclub.local/backend/internal/platform/config"
)

func TestHealthRoutes(t *testing.T) {
	var calls int
	a := newApp(testConfig(), testLogger(), func(context.Context) error {
		calls++
		return errors.New("database unavailable")
	}, func() {})
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/health/live", 200},
		{http.MethodGet, "/health/ready", 503},
		{http.MethodPost, "/health/live", 405},
		{http.MethodGet, "/api/v1/dogs", 404},
	} {
		w := httptest.NewRecorder()
		a.server.Handler.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s %s: status %d, headers %v", test.method, test.path, w.Code, w.Header())
		}
	}
	if calls != 1 {
		t.Fatalf("only readiness should call Ping: %d calls", calls)
	}
}

func TestShutdownDrainsRequestsBeforeClosingPool(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var closed atomic.Bool
	a := newApp(testConfig(), testLogger(), func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, func() { closed.Store(true) })
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, listener) }()
	client := testClient(t, listener)
	response := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://dogclub.test/health/ready")
		if err == nil {
			if resp.StatusCode != http.StatusServiceUnavailable {
				err = errors.New("in-flight readiness did not observe shutdown")
			}
			_, copyErr := io.Copy(io.Discard, resp.Body)
			if err == nil {
				err = copyErr
			}
			_ = resp.Body.Close()
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter Ping")
	}
	cancel()
	// Listener closure proves shutdown started while the request is still active.
	select {
	case <-listener.closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not stop accepting connections")
	}
	if closed.Load() {
		t.Error("pool was closed before the active request finished")
	}
	close(release)
	select {
	case err := <-response:
		if err != nil {
			t.Errorf("request failed during graceful shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not finish")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
	if !closed.Load() {
		t.Fatal("pool was not closed")
	}
	if conn, err := listener.dialContext(context.Background(), "", ""); err == nil {
		_ = conn.Close()
		t.Fatal("server is still accepting connections")
	}
}

func TestShutdownDeadlineCancelsRequestsAndClosesPool(t *testing.T) {
	entered := make(chan struct{})
	requestCanceled := make(chan struct{})
	var closed atomic.Bool
	cfg := testConfig()
	cfg.ShutdownTimeout = 30 * time.Millisecond
	a := newApp(cfg, testLogger(), func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(requestCanceled)
		return ctx.Err()
	}, func() { closed.Store(true) })
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, listener) }()
	response := make(chan struct{})
	client := testClient(t, listener)
	go func() {
		defer close(response)
		resp, err := client.Get("http://dogclub.test/health/ready")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected shutdown deadline, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown exceeded its deadline")
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("in-flight request was not canceled")
	}
	if !closed.Load() {
		t.Fatal("pool was not closed")
	}
	<-response
}

func TestListenFailureClosesPool(t *testing.T) {
	cfg := testConfig()
	cfg.HTTPAddr = "missing-port"
	closed := false
	a := newApp(cfg, testLogger(), func(context.Context) error { return nil }, func() { closed = true })
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("expected a listen error for an invalid address")
	}
	if !closed {
		t.Fatal("pool leaked after listen failure")
	}
}

func testConfig() config.Config {
	return config.Config{
		HTTPAddr: "127.0.0.1:8080", DBPingTimeout: 5 * time.Second,
		ShutdownTimeout: 2 * time.Second, ReadHeaderTimeout: time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: time.Second,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// Exercise the real HTTP server and client over net.Pipe without OS sockets.
type memoryListener struct {
	incoming chan net.Conn
	closed   chan struct{}
	once     sync.Once
}

func testListener(t *testing.T) *memoryListener {
	t.Helper()
	listener := &memoryListener{incoming: make(chan net.Conn), closed: make(chan struct{})}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func (l *memoryListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	case conn := <-l.incoming:
		return conn, nil
	}
}

func (l *memoryListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *memoryListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (l *memoryListener) dialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case <-l.closed:
		_ = server.Close()
		_ = client.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = server.Close()
		_ = client.Close()
		return nil, ctx.Err()
	case l.incoming <- server:
		return client, nil
	}
}

func testClient(t *testing.T, listener *memoryListener) *http.Client {
	t.Helper()
	transport := &http.Transport{DialContext: listener.dialContext}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func TestShutdownJoinsNotificationWorkerBeforePoolClose(t *testing.T) {
	started := make(chan struct{})
	var stopped, closed atomic.Bool
	a := newApp(testConfig(), testLogger(), func(context.Context) error { return nil }, func() {
		if !stopped.Load() {
			t.Error("pool closed before notification worker")
		}
		closed.Store(true)
	})
	a.worker = func(ctx context.Context, _ *slog.Logger) { close(started); <-ctx.Done(); stopped.Store(true) }
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, listener) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join worker")
	}
	if !closed.Load() {
		t.Fatal("pool not closed")
	}
}
