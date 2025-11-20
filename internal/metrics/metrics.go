package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/thealonlevi/subnet-sentinel/internal/checker"
)

type Registry struct {
	mu              sync.RWMutex
	lastRunTime     time.Time
	lastRunDuration time.Duration
	totalRuns       uint64
	totalSuccess    uint64
	totalFailure    uint64
	mountAttempts   uint64
	subnetSuccess   map[string]uint64
	subnetFailure   map[string]uint64
	targetSuccess   map[string]uint64
	targetFailure   map[string]uint64
	hostFailures    map[string]uint64
}

func NewRegistry() *Registry {
	return &Registry{
		subnetSuccess: make(map[string]uint64),
		subnetFailure: make(map[string]uint64),
		targetSuccess: make(map[string]uint64),
		targetFailure: make(map[string]uint64),
		hostFailures:  make(map[string]uint64),
	}
}

func (r *Registry) ObserveRun(start time.Time, duration time.Duration, results []checker.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalRuns++
	r.lastRunTime = start.Add(duration)
	r.lastRunDuration = duration

	for _, res := range results {
		if res.Success {
			r.totalSuccess++
			r.subnetSuccess[res.Subnet]++
			r.targetSuccess[res.URL]++
		} else {
			r.totalFailure++
			r.subnetFailure[res.Subnet]++
			r.targetFailure[res.URL]++
			key := res.Subnet + "|" + res.SourceIP
			r.hostFailures[key]++
		}
	}
}

func (r *Registry) IncMountAttempt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mountAttempts++
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		fmt.Fprintf(w, "# HELP subnet_sentinel_last_run_timestamp_seconds Timestamp of last run\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_last_run_timestamp_seconds gauge\n")
		if !r.lastRunTime.IsZero() {
			fmt.Fprintf(w, "subnet_sentinel_last_run_timestamp_seconds %.3f\n", float64(r.lastRunTime.Unix())+float64(r.lastRunTime.Nanosecond())/1e9)
		} else {
			fmt.Fprintf(w, "subnet_sentinel_last_run_timestamp_seconds 0\n")
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_run_duration_seconds Duration of last run in seconds\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_run_duration_seconds gauge\n")
		fmt.Fprintf(w, "subnet_sentinel_run_duration_seconds %.3f\n", r.lastRunDuration.Seconds())

		fmt.Fprintf(w, "# HELP subnet_sentinel_runs_total Total number of runs\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_runs_total counter\n")
		fmt.Fprintf(w, "subnet_sentinel_runs_total %d\n", r.totalRuns)

		fmt.Fprintf(w, "# HELP subnet_sentinel_requests_success_total Total successful requests\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_requests_success_total counter\n")
		fmt.Fprintf(w, "subnet_sentinel_requests_success_total %d\n", r.totalSuccess)

		fmt.Fprintf(w, "# HELP subnet_sentinel_requests_failure_total Total failed requests\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_requests_failure_total counter\n")
		fmt.Fprintf(w, "subnet_sentinel_requests_failure_total %d\n", r.totalFailure)

		fmt.Fprintf(w, "# HELP subnet_sentinel_subnet_requests_success_total Successful requests per subnet\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_subnet_requests_success_total counter\n")
		for cidr, count := range r.subnetSuccess {
			fmt.Fprintf(w, "subnet_sentinel_subnet_requests_success_total{cidr=\"%s\"} %d\n", cidr, count)
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_subnet_requests_failure_total Failed requests per subnet\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_subnet_requests_failure_total counter\n")
		for cidr, count := range r.subnetFailure {
			fmt.Fprintf(w, "subnet_sentinel_subnet_requests_failure_total{cidr=\"%s\"} %d\n", cidr, count)
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_target_requests_success_total Successful requests per target\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_target_requests_success_total counter\n")
		for target, count := range r.targetSuccess {
			fmt.Fprintf(w, "subnet_sentinel_target_requests_success_total{target=\"%s\"} %d\n", target, count)
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_target_requests_failure_total Failed requests per target\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_target_requests_failure_total counter\n")
		for target, count := range r.targetFailure {
			fmt.Fprintf(w, "subnet_sentinel_target_requests_failure_total{target=\"%s\"} %d\n", target, count)
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_host_failures_total Failures per host\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_host_failures_total counter\n")
		for key, count := range r.hostFailures {
			parts := splitHostKey(key)
			if len(parts) == 2 {
				fmt.Fprintf(w, "subnet_sentinel_host_failures_total{subnet=\"%s\",ip=\"%s\"} %d\n", parts[0], parts[1], count)
			}
		}

		fmt.Fprintf(w, "# HELP subnet_sentinel_mount_attempts_total Total mount attempts\n")
		fmt.Fprintf(w, "# TYPE subnet_sentinel_mount_attempts_total counter\n")
		fmt.Fprintf(w, "subnet_sentinel_mount_attempts_total %d\n", r.mountAttempts)
	})
}

func splitHostKey(key string) []string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '|' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}

func ListenAndServe(addr string, registry *Registry) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", registry.Handler())
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server.ListenAndServe()
}
