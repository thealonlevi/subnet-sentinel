package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/checker"
	"github.com/thealonlevi/subnet-sentinel/internal/config"
	"github.com/thealonlevi/subnet-sentinel/internal/httpclient"
	"github.com/thealonlevi/subnet-sentinel/internal/logging"
	"github.com/thealonlevi/subnet-sentinel/internal/metrics"
	"github.com/thealonlevi/subnet-sentinel/internal/mount"
	"github.com/thealonlevi/subnet-sentinel/internal/scripts"
	"github.com/thealonlevi/subnet-sentinel/internal/subnets"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var configPath string
	var logLevel string
	var logFormat string
	var metricsAddr string
	flags := flag.NewFlagSet("subnet-sentinel", flag.ContinueOnError)
	flags.StringVar(&configPath, "config", "", "")
	flags.StringVar(&configPath, "c", "", "")
	flags.StringVar(&logLevel, "log-level", "info", "")
	flags.StringVar(&logFormat, "log-format", "", "")
	flags.StringVar(&metricsAddr, "metrics-addr", "", "")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if configPath == "" {
		if env := os.Getenv("SUBNET_SENTINEL_CONFIG"); env != "" {
			configPath = env
		} else {
			configPath = "config.yaml"
		}
	}
	if logLevel == "" {
		logLevel = "info"
	}
	args := flags.Args()
	command := "run"
	if len(args) > 0 {
		command = strings.ToLower(args[0])
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if logFormat == "" {
		logFormat = cfg.LogFormat
	}
	if metricsAddr == "" {
		metricsAddr = cfg.MetricsAddr
	}
	logger, err := logging.New(logLevel, logFormat)
	if err != nil {
		return err
	}
	var reg *metrics.Registry
	if metricsAddr != "" {
		reg = metrics.NewRegistry()
		go func() {
			if err := metrics.ListenAndServe(metricsAddr, reg); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server error: %v", err)
			}
		}()
	}
	subnetDefs, err := subnets.FromConfigs(cfg.Subnets)
	if err != nil {
		return err
	}
	requests := mount.PrepareRequests(cfg.DefaultInterface, subnetDefs)
	var mountFn func(context.Context) error
	if cfg.AutoMountSubnets {
		mountFn = func(runCtx context.Context) error {
			if reg != nil {
				reg.IncMountAttempt()
			}
			logger.Info("auto mount triggered")
			statuses, err := mount.EnsureMounted(runCtx, requests)
			for _, status := range statuses {
				logger.Info("mount status subnet=%s interface=%s ip_assigned=%t route=%t nonlocal=%t", status.CIDR, status.Interface, status.IPAssigned, status.RouteExists, status.NonLocalBind)
				if len(status.Errors) > 0 {
					logger.Error("mount errors subnet=%s errors=%s", status.CIDR, strings.Join(status.Errors, "; "))
				}
			}
			return err
		}
	}
	var onFailure checker.OnFailureFunc
	if cfg.RunFailureScripts {
		scriptRunner, err := scripts.NewRunner(cfg.FailureScriptsDir, logger, 10*time.Second)
		if err != nil {
			logger.Warn("failed to initialize failure scripts err=%v", err)
		} else {
			onFailure = func(ctx context.Context, e checker.FailureEvent) {
				scriptRunner.OnFailure(ctx, scripts.FailureEvent{
					SubnetCIDR: e.SubnetCIDR,
					IP:         e.IP,
					Target:     e.Target,
					Error:      e.Error,
					Timestamp:  e.Timestamp,
				})
			}
		}
	}
	switch command {
	case "run":
		return executeRunLoop(ctx, cfg, subnetDefs, logger, mountFn, onFailure, reg)
	case "once":
		return executeOnce(ctx, cfg, subnetDefs, logger, mountFn, onFailure, reg)
	case "check-mount":
		return executeCheckMount(ctx, logger, requests)
	case "mount":
		return executeMount(ctx, logger, requests, reg)
	case "":
		return executeRunLoop(ctx, cfg, subnetDefs, logger, mountFn, onFailure, reg)
	default:
		return fmt.Errorf("unknown command %s", command)
	}
}

func executeRunLoop(ctx context.Context, cfg config.Config, subs []subnets.Subnet, logger logging.Logger, mountFn func(context.Context) error, onFailure checker.OnFailureFunc, reg *metrics.Registry) error {
	client := httpclient.New(15 * time.Second)
	chk, err := checker.New(cfg, subs, client, logger, mountFn, onFailure)
	if err != nil {
		return err
	}
	rawInterval := cfg.IntervalSeconds
	var interval time.Duration
	if rawInterval == 0 {
		logger.Info("intervalSeconds is 0; using 1s interval to avoid busy-loop")
		interval = time.Second
	} else {
		interval = time.Duration(rawInterval) * time.Second
	}
	runID := 1
	for {
		start := time.Now()
		results, err := chk.Run(ctx)
		if err != nil {
			return ensureRunErrorHandled(err)
		}
		elapsed := time.Since(start)
		if reg != nil {
			reg.ObserveRun(start, elapsed, results)
		}
		printSummary(runID, results)
		runID++
		sleep := interval - elapsed
		if sleep > 0 {
			select {
			case <-ctx.Done():
				return ensureRunErrorHandled(ctx.Err())
			case <-time.After(sleep):
			}
		}
	}
}

func executeOnce(ctx context.Context, cfg config.Config, subs []subnets.Subnet, logger logging.Logger, mountFn func(context.Context) error, onFailure checker.OnFailureFunc, reg *metrics.Registry) error {
	client := httpclient.New(15 * time.Second)
	chk, err := checker.New(cfg, subs, client, logger, mountFn, onFailure)
	if err != nil {
		return err
	}
	start := time.Now()
	results, err := chk.Run(ctx)
	if err != nil {
		return ensureRunErrorHandled(err)
	}
	duration := time.Since(start)
	if reg != nil {
		reg.ObserveRun(start, duration, results)
	}
	printSummary(1, results)
	return nil
}

func executeCheckMount(ctx context.Context, logger logging.Logger, requests []mount.Request) error {
	statuses, err := mount.Check(ctx, requests)
	if err != nil {
		return err
	}
	printMountStatuses("CHECK", statuses)
	return nil
}

func executeMount(ctx context.Context, logger logging.Logger, requests []mount.Request, reg *metrics.Registry) error {
	if reg != nil {
		reg.IncMountAttempt()
	}
	statuses, err := mount.EnsureMounted(ctx, requests)
	if err != nil {
		return err
	}
	printMountStatuses("MOUNT", statuses)
	return nil
}

func ensureRunErrorHandled(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func printSummary(runID int, results []checker.Result) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Printf("RUN %d %s total=%d\n", runID, timestamp, len(results))
	for _, res := range results {
		status := "OK"
		detail := fmt.Sprintf("status=%d", res.StatusCode)
		if !res.Success {
			status = "FAIL"
			if res.Error != "" {
				detail = res.Error
			} else {
				detail = "error"
			}
		}
		duration := res.Duration.Truncate(time.Millisecond)
		fmt.Printf("%s subnet=%s ip=%s url=%s duration=%s %s\n", status, res.Subnet, res.SourceIP, res.URL, duration.String(), detail)
	}
}

func printMountStatuses(prefix string, statuses []mount.Status) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Printf("%s %s total=%d\n", prefix, timestamp, len(statuses))
	for _, status := range statuses {
		ipAssigned := "no"
		if status.IPAssigned {
			ipAssigned = "yes"
		}
		route := "no"
		if status.RouteExists {
			route = "yes"
		}
		nonLocal := "no"
		if status.NonLocalBind {
			nonLocal = "yes"
		}
		mountIP := ""
		if status.MountIP != nil {
			mountIP = status.MountIP.String()
		}
		fmt.Printf("subnet=%s interface=%s ip_assigned=%s route=%s nonlocal=%s mount_ip=%s\n", status.CIDR, status.Interface, ipAssigned, route, nonLocal, mountIP)
		if len(status.Actions) > 0 {
			fmt.Printf(" actions=%s\n", strings.Join(status.Actions, "; "))
		}
		if len(status.Errors) > 0 {
			fmt.Printf(" errors=%s\n", strings.Join(status.Errors, "; "))
		}
	}
}
