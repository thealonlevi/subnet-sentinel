package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/checker"
)

func TestRegistryObserveRun(t *testing.T) {
	reg := NewRegistry()
	start := time.Now()
	results := []checker.Result{
		{Subnet: "192.168.1.0/24", SourceIP: "192.168.1.1", URL: "https://test1.com", Success: true, StatusCode: 200},
		{Subnet: "192.168.1.0/24", SourceIP: "192.168.1.1", URL: "https://test2.com", Success: false, StatusCode: 500},
		{Subnet: "192.168.2.0/24", SourceIP: "192.168.2.1", URL: "https://test1.com", Success: true, StatusCode: 200},
	}
	duration := 100 * time.Millisecond
	reg.ObserveRun(start, duration, results)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	reg.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "subnet_sentinel_runs_total 1") {
		t.Errorf("expected runs_total to be 1, body: %s", body)
	}
	if !strings.Contains(body, "subnet_sentinel_requests_success_total 2") {
		t.Errorf("expected success_total to be 2, body: %s", body)
	}
	if !strings.Contains(body, "subnet_sentinel_requests_failure_total 1") {
		t.Errorf("expected failure_total to be 1, body: %s", body)
	}
	if !strings.Contains(body, `subnet_sentinel_subnet_requests_success_total{cidr="192.168.1.0/24"}`) {
		t.Errorf("expected subnet success metric, body: %s", body)
	}
	if !strings.Contains(body, `subnet_sentinel_target_requests_success_total{target="https://test1.com"}`) {
		t.Errorf("expected target success metric, body: %s", body)
	}
	if !strings.Contains(body, `subnet_sentinel_host_failures_total{subnet="192.168.1.0/24",ip="192.168.1.1"}`) {
		t.Errorf("expected host failure metric, body: %s", body)
	}
}

func TestRegistryIncMountAttempt(t *testing.T) {
	reg := NewRegistry()
	reg.IncMountAttempt()
	reg.IncMountAttempt()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	reg.Handler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "subnet_sentinel_mount_attempts_total 2") {
		t.Errorf("expected mount_attempts_total to be 2, body: %s", body)
	}
}
