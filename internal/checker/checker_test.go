package checker

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/config"
	"github.com/thealonlevi/subnet-sentinel/internal/httpclient"
	"github.com/thealonlevi/subnet-sentinel/internal/logging"
	"github.com/thealonlevi/subnet-sentinel/internal/subnets"
)

type mockHTTPClient struct {
	responses []mockResponse
	calls     []callRecord
}

type mockResponse struct {
	result httpclient.Result
	err    error
}

type callRecord struct {
	IP  net.IP
	URL string
}

func (m *mockHTTPClient) Do(ctx context.Context, source net.IP, url string) (httpclient.Result, error) {
	if len(m.responses) == 0 {
		return httpclient.Result{}, fmt.Errorf("no mock response configured")
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	m.calls = append(m.calls, callRecord{IP: append(net.IP(nil), source...), URL: url})
	return resp.result, resp.err
}

func TestCheckerRunCollectsResults(t *testing.T) {
	cfg := config.Config{
		Subnets: []config.SubnetConfig{
			{CIDR: "192.168.50.0/30"},
		},
		Targets:      []string{"https://success.test", "https://failure.test"},
		IPsPerSubnet: 1,
	}
	subs, err := subnets.FromConfigs(cfg.Subnets)
	if err != nil {
		t.Fatalf("subnet parse: %v", err)
	}
	mock := &mockHTTPClient{
		responses: []mockResponse{
			{result: httpclient.Result{StatusCode: 200, Duration: 50 * time.Millisecond}},
			{result: httpclient.Result{StatusCode: 503, Duration: 80 * time.Millisecond}, err: fmt.Errorf("service unavailable")},
		},
	}
	logger, err := logging.New("error")
	if err != nil {
		t.Fatalf("logger init: %v", err)
	}
	chk, err := New(cfg, subs, mock, logger)
	if err != nil {
		t.Fatalf("checker init: %v", err)
	}
	results, err := chk.Run(context.Background())
	if err != nil {
		t.Fatalf("checker run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	successCount := 0
	failCount := 0
	for _, res := range results {
		if res.Subnet != cfg.Subnets[0].CIDR {
			t.Fatalf("unexpected subnet %s", res.Subnet)
		}
		if res.Duration <= 0 {
			t.Fatalf("expected duration set")
		}
		if res.Success {
			successCount++
			if res.StatusCode != 200 {
				t.Fatalf("expected 200 status, got %d", res.StatusCode)
			}
			if res.Error != "" {
				t.Fatalf("expected empty error, got %s", res.Error)
			}
		} else {
			failCount++
			if res.StatusCode != 503 {
				t.Fatalf("expected 503 status, got %d", res.StatusCode)
			}
			if res.Error == "" {
				t.Fatalf("expected error message")
			}
		}
		if parsed := net.ParseIP(res.SourceIP); parsed == nil {
			t.Fatalf("invalid source ip %s", res.SourceIP)
		}
	}
	if successCount != 1 || failCount != 1 {
		t.Fatalf("unexpected success/failure counts: %d/%d", successCount, failCount)
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 client calls, got %d", len(mock.calls))
	}
	if mock.calls[0].IP.String() != mock.calls[1].IP.String() {
		t.Fatalf("expected same host ip for both calls")
	}
}
