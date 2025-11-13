package httpclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Result struct {
	StatusCode int
	Duration   time.Duration
}

type Client struct {
	Timeout time.Duration
}

func New(timeout time.Duration) *Client {
	return &Client{Timeout: timeout}
}

func (c *Client) Do(ctx context.Context, source net.IP, url string) (Result, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ip4 := source.To4()
	if ip4 == nil {
		return Result{}, fmt.Errorf("source ip must be ipv4")
	}
	dialer := &net.Dialer{
		Timeout:   timeout,
		LocalAddr: &net.TCPAddr{IP: ip4, Port: 0},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          0,
		MaxConnsPerHost:       0,
		MaxIdleConnsPerHost:   0,
		DisableCompression:    true,
		ResponseHeaderTimeout: timeout,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Connection", "close")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	duration := time.Since(start)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{StatusCode: resp.StatusCode, Duration: duration}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return Result{StatusCode: resp.StatusCode, Duration: duration}, nil
}
