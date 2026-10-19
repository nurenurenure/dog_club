package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestCookieAndRefreshTransport(t *testing.T) {
	h := NewHandler(nil, true)
	w := httptest.NewRecorder()
	h.writeCredentials(w, Credentials{AccessToken: "access-secret", Refresh: "refresh-secret", RefreshUntil: time.Now().Add(time.Hour), TokenType: "Bearer", ExpiresIn: 600})
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != cookiePath || cookies[0].MaxAge < 1 {
		t.Fatal("refresh cookie is not protected")
	}
	if strings.Contains(w.Body.String(), "refresh-secret") {
		t.Fatal("refresh verifier exposed in JSON")
	}
	w = httptest.NewRecorder()
	h.clearCookie(w)
	if w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("refresh cookie not removed")
	}
}

func TestAuthenticationAndCSRFRejectBeforeService(t *testing.T) {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	NewHandler(nil, false).Register(router)
	for _, test := range []struct {
		method, path, origin, fetch string
		status                      int
	}{
		{"GET", "/api/v1/users", "", "", 401},
		{"POST", "/api/v1/auth/login", "https://untrusted.example", "cross-site", 403},
		{"POST", "/api/v1/auth/refresh", "https://untrusted.example", "cross-site", 403},
		{"POST", "/api/v1/auth/logout", "https://untrusted.example", "cross-site", 403},
		{"POST", "/api/v1/auth/refresh", "", "", 401},
		{"POST", "/api/v1/auth/activate", "", "", 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(test.method, "http://dogclub.test"+test.path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Sec-Fetch-Site", test.fetch)
		// Invalid token is rejected before reaching a nil service.
		if strings.HasSuffix(test.path, "activate") {
			r.Body = http.NoBody
		}
		router.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s: got %d, want %d", test.path, w.Code, test.status)
		}
		var p struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.RequestID == "" || p.RequestID != w.Header().Get("X-Request-ID") {
			t.Fatal("inconsistent problem response")
		}
	}
}

func TestLimiterBoundsAndExpiration(t *testing.T) {
	now := time.Now()
	l := limiter{entries: make(map[string]window), now: func() time.Time { return now }}
	if !l.allow("client", 2, time.Minute) || !l.allow("client", 2, time.Minute) || l.allow("client", 2, time.Minute) {
		t.Fatal("throttle not enforced")
	}
	now = now.Add(time.Minute)
	if !l.allow("client", 2, time.Minute) {
		t.Fatal("throttle did not expire")
	}
	for i := 0; i < 4096; i++ {
		l.entries[string(rune(i))+"key"] = window{start: now}
	}
	if l.allow("new-client", 2, time.Minute) {
		t.Fatal("limiter map is unbounded")
	}
	now = now.Add(16 * time.Minute)
	if !l.allow("new-client", 2, time.Minute) {
		t.Fatal("stale clients not removed")
	}
}

func TestInactiveAndDifferentProfilesDoNotGrantAdmin(t *testing.T) {
	for _, p := range []Principal{{Profile: Profile{Role: "admin"}, ProfileValid: false}, {Profile: Profile{Role: "owner"}, ProfileValid: true}} {
		if RequireProfile(p, "admin") == nil {
			t.Fatal("inactive or unrelated profile granted admin")
		}
	}
}
