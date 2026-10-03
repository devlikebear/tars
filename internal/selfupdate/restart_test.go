package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer answers /v1/healthz with oldVersion until a restart, then with
// newVersion after a few probes, as a re-executing server does.
func fakeServer(t *testing.T, oldVersion, newVersion, wantToken string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var restarted atomic.Bool
	var probesAfter, restarts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/admin/restart":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if wantToken != "" && r.Header.Get("Authorization") != "Bearer "+wantToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			restarts.Add(1)
			restarted.Store(true)
			_, _ = w.Write([]byte(`{"ok":"true","mode":"exec"}`))
		case "/v1/healthz":
			v := oldVersion
			if restarted.Load() && probesAfter.Add(1) > 2 {
				v = newVersion
			}
			_, _ = fmt.Fprintf(w, `{"ok":true,"component":"tars","version":%q}`, v)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &restarts
}

func TestRestartWaitsForTheNewVersion(t *testing.T) {
	srv, restarts := fakeServer(t, "1.2.3", "1.3.0", "admin-secret")
	s := Server{URL: srv.URL, Token: "admin-secret", HTTPClient: srv.Client(), PollEvery: time.Millisecond}

	if v, err := s.Version(context.Background()); err != nil || v != "1.2.3" {
		t.Fatalf("version = %q, %v", v, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Restart(ctx, "1.3.0"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if restarts.Load() != 1 {
		t.Fatalf("restarts = %d", restarts.Load())
	}
}

func TestRestartReportsAServerThatKeepsItsOldVersion(t *testing.T) {
	srv, _ := fakeServer(t, "1.2.3", "1.2.3", "")
	s := Server{URL: srv.URL, HTTPClient: srv.Client(), PollEvery: time.Millisecond}
	// Long enough for a slow Windows runner to get a probe in after the
	// restart; the server never reports 1.3.0, so the test always waits it out.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := s.Restart(ctx, "1.3.0")
	if err == nil || err.Error() != "the server came back as 1.2.3, not 1.3.0; was it started from another tars?" {
		t.Fatalf("err = %v", err)
	}
}

func TestRestartNeedsTheAdminToken(t *testing.T) {
	srv, restarts := fakeServer(t, "1.2.3", "1.3.0", "admin-secret")
	s := Server{URL: srv.URL, Token: "user-token", HTTPClient: srv.Client(), PollEvery: time.Millisecond}
	if err := s.Restart(context.Background(), "1.3.0"); !errors.Is(err, ErrRestartUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if restarts.Load() != 0 {
		t.Fatal("restart ran without the token")
	}
}

func TestVersionWithoutAServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	s := Server{URL: url, HTTPClient: &http.Client{Timeout: time.Second}}
	if _, err := s.Version(context.Background()); !errors.Is(err, ErrServerNotRunning) {
		t.Fatalf("err = %v", err)
	}
	// Something else on the port is not a tars server either.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"component":"other"}`))
	}))
	defer other.Close()
	if _, err := (Server{URL: other.URL, HTTPClient: other.Client()}).Version(context.Background()); !errors.Is(err, ErrServerNotRunning) {
		t.Fatalf("other component: err = %v", err)
	}
}
