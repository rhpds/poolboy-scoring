package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestBasicAuthFilterProvider_ValidCredentials(t *testing.T) {
	handler := applyFilter(t, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("admin", "secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("got body %q, want %q", rr.Body.String(), "ok")
	}
}

func TestBasicAuthFilterProvider_InvalidPassword(t *testing.T) {
	handler := applyFilter(t, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("admin", "wrong")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Error("missing WWW-Authenticate header")
	}
}

func TestBasicAuthFilterProvider_InvalidUsername(t *testing.T) {
	handler := applyFilter(t, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("wrong", "secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestBasicAuthFilterProvider_NoAuthHeader(t *testing.T) {
	handler := applyFilter(t, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestBasicAuthFilterProvider_EmptyCredentials(t *testing.T) {
	handler := applyFilter(t, "admin", "secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// applyFilter creates a filter from BasicAuthFilterProvider and wraps a
// simple "ok" handler, returning the protected handler for testing.
func applyFilter(t *testing.T, username, password string) http.Handler {
	t.Helper()

	provider := BasicAuthFilterProvider(username, password)
	filter, err := provider(nil, nil)
	if err != nil {
		t.Fatalf("FilterProvider returned error: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler, err := filter(logr.Discard(), inner)
	if err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	return handler
}
