package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiveDoesNotPing(t *testing.T) {
	h := New(func(context.Context) error {
		t.Fatal("liveness must not query PostgreSQL")
		return nil
	}, time.Second)
	h.Stop()
	check(t, h.Live, http.StatusOK, "ok")
}

func TestReadyTracksDatabaseRecovery(t *testing.T) {
	available := false
	h := New(func(context.Context) error {
		if !available {
			return errors.New("postgres://user:private_password@localhost/db")
		}
		return nil
	}, time.Second)
	check(t, h.Ready, http.StatusServiceUnavailable, "not_ready")
	available = true
	check(t, h.Ready, http.StatusOK, "ready")
	available = false
	check(t, h.Ready, http.StatusServiceUnavailable, "not_ready")
}

func TestReadyTimesOut(t *testing.T) {
	h := New(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("ping context has no deadline")
		}
		<-ctx.Done()
		return ctx.Err()
	}, 10*time.Millisecond)
	started := time.Now()
	check(t, h.Ready, http.StatusServiceUnavailable, "not_ready")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("ping timeout was not enforced: %s", elapsed)
	}
}

func TestReadyHonorsRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := New(func(ctx context.Context) error { return ctx.Err() }, time.Second)
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil).WithContext(ctx))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("canceled request returned %d", w.Code)
	}
}

func TestStoppedReadinessDoesNotPing(t *testing.T) {
	h := New(func(context.Context) error {
		t.Fatal("readiness must not query the pool after shutdown starts")
		return nil
	}, time.Second)
	h.Stop()
	check(t, h.Ready, http.StatusServiceUnavailable, "not_ready")
}

func TestStopDuringPingWithdrawsReadiness(t *testing.T) {
	var h *Handler
	h = New(func(context.Context) error { h.Stop(); return nil }, time.Second)
	check(t, h.Ready, http.StatusServiceUnavailable, "not_ready")
}

func check(t *testing.T, handler http.HandlerFunc, code int, status string) {
	t.Helper()
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if w.Code != code || w.Body.String() != `{"status":"`+status+`"}`+"\n" {
		t.Fatalf("response: status %d, body %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("incorrect health headers: %v", w.Header())
	}
}
