package scripts

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type FailureEvent struct {
	SubnetCIDR string
	IP         net.IP
	Target     string
	Error      error
	Timestamp  time.Time
}

type Logger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

type Runner struct {
	scripts []string
	logger  Logger
	timeout time.Duration
}

func NewRunner(dir string, logger Logger, timeout time.Duration) (*Runner, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("failure scripts directory does not exist, skipping", "dir", dir)
			return &Runner{scripts: nil, logger: logger, timeout: timeout}, nil
		}
		return nil, fmt.Errorf("read failure scripts dir: %w", err)
	}

	var scripts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".sh" {
			continue
		}
		scripts = append(scripts, filepath.Join(dir, name))
	}

	sort.Strings(scripts)

	logger.Info("loaded failure scripts", "dir", dir, "count", len(scripts))

	return &Runner{
		scripts: scripts,
		logger:  logger,
		timeout: timeout,
	}, nil
}

func (r *Runner) OnFailure(ctx context.Context, e FailureEvent) {
	if len(r.scripts) == 0 {
		return
	}

	for _, path := range r.scripts {
		r.runOne(ctx, path, e)
	}
}

func (r *Runner) runOne(parentCtx context.Context, path string, e FailureEvent) {
	ctx := parentCtx
	var cancel context.CancelFunc
	if r.timeout > 0 {
		ctx, cancel = context.WithTimeout(parentCtx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/env", "bash", path)
	cmd.Env = append(os.Environ(),
		"SUBNET_CIDR="+e.SubnetCIDR,
		"IP="+e.IP.String(),
		"TARGET="+e.Target,
		"ERROR="+safeErr(e.Error),
		"TIMESTAMP="+e.Timestamp.Format(time.RFC3339),
	)

	if err := cmd.Run(); err != nil {
		r.logger.Warn("failure script error", "script", path, "err", err)
	} else {
		r.logger.Info("failure script ran", "script", path)
	}
}

func safeErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
