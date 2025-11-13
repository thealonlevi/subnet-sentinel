package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/checker"
	"github.com/thealonlevi/subnet-sentinel/internal/config"
	"github.com/thealonlevi/subnet-sentinel/internal/httpclient"
	"github.com/thealonlevi/subnet-sentinel/internal/logging"
	"github.com/thealonlevi/subnet-sentinel/internal/mount"
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
	flags := flag.NewFlagSet("subnet-sentinel", flag.ContinueOnError)
	flags.StringVar(&configPath, "config", "", "")
	flags.StringVar(&configPath, "c", "", "")
	flags.StringVar(&logLevel, "log-level", "info", "")
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
	logger, err := logging.New(logLevel)
	if err != nil {
		return err
	}
	subnetDefs, err := subnets.FromConfigs(cfg.Subnets)
	if err != nil {
		return err
	}
	requests := mount.PrepareRequests(cfg.DefaultInterface, subnetDefs)
	switch command {
	case "run":
		return executeRunLoop(ctx, cfg, subnetDefs, requests, logger)
	case "once":
		return executeOnce(ctx, cfg, subnetDefs, requests, logger)
	case "check-mount":
		return executeCheckMount(ctx, requests)
	case "mount":
		return executeMount(ctx, requests)
	case "":
		return executeRunLoop(ctx, cfg, subnetDefs, requests, logger)
	default:
		return fmt.Errorf("unknown command %s", command)
	}
}

func executeRunLoop(ctx context.Context, cfg config.Config, subs []subnets.Subnet, requests []mount.Request, logger logging.Logger) error {
	client := httpclient.New(15 * time.Second)
	chk, err := checker.New(cfg, subs, client, logger)
	if err != nil {
		return err
	}
	if cfg.AutoMountSubnets {
		if _, err := mount.EnsureMounted(ctx, requests); err != nil {
			logger.Error("auto mount failed: %v", err)
		}
	}
	interval := time.Duration(cfg.IntervalSeconds) * time.Second
	if interval < 0 {
		interval = 0
	}
	runID := 1
	for {
		start := time.Now()
		results, err := chk.Run(ctx)
		if err != nil {
			return ensureRunErrorHandled(err)
		}
		printSummary(runID, results)
		runID++
		if interval == 0 {
			continue
		}
		elapsed := time.Since(start)
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

func executeOnce(ctx context.Context, cfg config.Config, subs []subnets.Subnet, requests []mount.Request, logger logging.Logger) error {
	client := httpclient.New(15 * time.Second)
	chk, err := checker.New(cfg, subs, client, logger)
	if err != nil {
		return err
	}
	if cfg.AutoMountSubnets {
		if _, err := mount.EnsureMounted(ctx, requests); err != nil {
			logger.Error("auto mount failed: %v", err)
		}
	}
	results, err := chk.Run(ctx)
	if err != nil {
		return ensureRunErrorHandled(err)
	}
	printSummary(1, results)
	return nil
}

func executeCheckMount(ctx context.Context, requests []mount.Request) error {
	statuses, err := mount.Check(ctx, requests)
	if err != nil {
		return err
	}
	printMountStatuses("CHECK", statuses)
	return nil
}

func executeMount(ctx context.Context, requests []mount.Request) error {
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
