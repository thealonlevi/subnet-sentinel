package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/config"
	"github.com/thealonlevi/subnet-sentinel/internal/httpclient"
	"github.com/thealonlevi/subnet-sentinel/internal/logging"
	"github.com/thealonlevi/subnet-sentinel/internal/subnets"
)

type HTTPClient interface {
	Do(ctx context.Context, source net.IP, url string) (httpclient.Result, error)
}

type Checker struct {
	Config       config.Config
	Subnets      []subnets.Subnet
	Client       HTTPClient
	Logger       logging.Logger
	MountOnError func(context.Context) error
}

type Result struct {
	Subnet     string
	SourceIP   string
	URL        string
	Success    bool
	StatusCode int
	Duration   time.Duration
	Error      string
}

func New(cfg config.Config, subs []subnets.Subnet, client HTTPClient, logger logging.Logger, mountOnError func(context.Context) error) (*Checker, error) {
	if client == nil {
		return nil, fmt.Errorf("http client is required")
	}
	return &Checker{
		Config:       cfg,
		Subnets:      subs,
		Client:       client,
		Logger:       logger,
		MountOnError: mountOnError,
	}, nil
}

func (c *Checker) Run(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0)
	mountAttempted := false
	for _, subnet := range c.Subnets {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		hosts, err := subnets.RandomHosts(subnet.Network, subnet.ExcludeHosts, c.Config.IPsPerSubnet)
		if err != nil {
			return results, fmt.Errorf("select hosts for %s: %w", subnet.CIDR, err)
		}
		for _, host := range hosts {
			for _, target := range c.Config.Targets {
				select {
				case <-ctx.Done():
					return results, ctx.Err()
				default:
				}
				res, err := c.performRequest(ctx, subnet.CIDR, host, target)
				initialErr := err
				retried := false
				if err != nil && c.MountOnError != nil {
					if !mountAttempted {
						mountAttempted = true
						if mountErr := c.MountOnError(ctx); mountErr != nil {
							c.Logger.Error("mount attempt failed: %v", mountErr)
						} else {
							c.Logger.Info("mount attempt completed")
						}
					}
					select {
					case <-ctx.Done():
						return results, ctx.Err()
					default:
					}
					retried = true
					resRetry, retryErr := c.performRequest(ctx, subnet.CIDR, host, target)
					res = resRetry
					err = retryErr
				}
				results = append(results, res)
				if err != nil {
					if retried {
						c.Logger.Error("request failed after retry subnet=%s ip=%s url=%s initial_error=%s error=%s", subnet.CIDR, host.String(), target, errorString(initialErr), err.Error())
					} else {
						c.Logger.Error("request failed subnet=%s ip=%s url=%s error=%s", subnet.CIDR, host.String(), target, err.Error())
					}
				} else {
					if retried || initialErr != nil {
						c.Logger.Info("request succeeded after retry subnet=%s ip=%s url=%s status=%d initial_error=%s", subnet.CIDR, host.String(), target, res.StatusCode, errorString(initialErr))
					} else {
						c.Logger.Debug("request succeeded subnet=%s ip=%s url=%s status=%d", subnet.CIDR, host.String(), target, res.StatusCode)
					}
				}
			}
		}
	}
	return results, nil
}

func (c *Checker) performRequest(ctx context.Context, subnet string, ip net.IP, target string) (Result, error) {
	start := time.Now()
	res, err := c.Client.Do(ctx, ip, target)
	duration := res.Duration
	if duration == 0 {
		duration = time.Since(start)
	}
	result := Result{
		Subnet:     subnet,
		SourceIP:   ip.String(),
		URL:        target,
		Success:    err == nil,
		StatusCode: res.StatusCode,
		Duration:   duration,
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result, err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
