package nt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oneclickvirt/nt3/model"
)

const (
	ProvinceLatencyErrorDNS         = "dns"
	ProvinceLatencyErrorRefused     = "refused"
	ProvinceLatencyErrorTimeout     = "timeout"
	ProvinceLatencyErrorUnreachable = "unreachable"
	ProvinceLatencyErrorCanceled    = "canceled"
	ProvinceLatencyErrorUnknown     = "unknown"
)

// ProvinceLatencyDialFunc permits callers to provide source-address-aware
// dialing and keeps the probe deterministic in offline tests.
type ProvinceLatencyDialFunc func(context.Context, string, string) (net.Conn, error)

// ProvinceLatencyConfig controls a batch of TCP handshake latency probes.
// Callers choose the target slice; the executor never expands it to all
// provinces implicitly. Zero values use the standard profile.
type ProvinceLatencyConfig struct {
	Attempts    int
	Timeout     time.Duration
	Concurrency int
	Port        int
	DialContext ProvinceLatencyDialFunc
	Now         func() time.Time
}

// ProvinceLatencySample is one TCP connection attempt.
type ProvinceLatencySample struct {
	Attempt    int           `json:"attempt"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	ErrorClass string        `json:"error_class,omitempty"`
}

// ProvinceLatencyResult contains metrics for one province/carrier/IP target.
// Latency metrics are calculated from successful handshakes only.
type ProvinceLatencyResult struct {
	Target       model.ProvinceLatencyTarget `json:"target"`
	Port         int                         `json:"port"`
	Attempts     int                         `json:"attempts"`
	Successful   int                         `json:"successful"`
	Failed       int                         `json:"failed"`
	SuccessRate  float64                     `json:"success_rate"`
	LossPercent  float64                     `json:"loss_percent"`
	Min          time.Duration               `json:"min"`
	Mean         time.Duration               `json:"mean"`
	P50          time.Duration               `json:"p50"`
	P95          time.Duration               `json:"p95"`
	Max          time.Duration               `json:"max"`
	Samples      []ProvinceLatencySample     `json:"samples"`
	ErrorCounts  map[string]int              `json:"error_counts,omitempty"`
	InvalidInput string                      `json:"invalid_input,omitempty"`
}

// StandardProvinceLatencyConfig favors a short standard report.
func StandardProvinceLatencyConfig() ProvinceLatencyConfig {
	return ProvinceLatencyConfig{
		Attempts:    2,
		Timeout:     1500 * time.Millisecond,
		Concurrency: 12,
		Port:        80,
		DialContext: (&net.Dialer{}).DialContext,
		Now:         time.Now,
	}
}

// DeepProvinceLatencyConfig increases the samples and timeout for deep runs.
func DeepProvinceLatencyConfig() ProvinceLatencyConfig {
	return ProvinceLatencyConfig{
		Attempts:    4,
		Timeout:     3 * time.Second,
		Concurrency: 16,
		Port:        80,
		DialContext: (&net.Dialer{}).DialContext,
		Now:         time.Now,
	}
}

func (config ProvinceLatencyConfig) withDefaults() ProvinceLatencyConfig {
	defaults := StandardProvinceLatencyConfig()
	if config.Attempts <= 0 {
		config.Attempts = defaults.Attempts
	}
	if config.Timeout <= 0 {
		config.Timeout = defaults.Timeout
	}
	if config.Concurrency <= 0 {
		config.Concurrency = defaults.Concurrency
	}
	if config.Port == 0 {
		config.Port = defaults.Port
	}
	if config.DialContext == nil {
		config.DialContext = defaults.DialContext
	}
	if config.Now == nil {
		config.Now = defaults.Now
	}
	return config
}

// RunProvinceLatencyTarget probes one target repeatedly. Network failures are
// represented in the returned result; only invalid input returns an error.
func RunProvinceLatencyTarget(ctx context.Context, target model.ProvinceLatencyTarget, config ProvinceLatencyConfig) (ProvinceLatencyResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config = config.withDefaults()
	result := newProvinceLatencyResult(target, config)
	if config.Port < 1 || config.Port > 65535 {
		return result, fmt.Errorf("province latency port %d is invalid", config.Port)
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return result, errors.New("province latency target host is empty")
	}
	network, err := provinceLatencyNetwork(target.IPVersion)
	if err != nil {
		return result, err
	}
	address := net.JoinHostPort(host, strconv.Itoa(config.Port))
	for attempt := 1; attempt <= config.Attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			result.recordFailure(attempt, classifyProvinceLatencyError(err))
			for remaining := attempt + 1; remaining <= config.Attempts; remaining++ {
				result.recordFailure(remaining, ProvinceLatencyErrorCanceled)
			}
			break
		}

		attemptCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		started := config.Now()
		conn, dialErr := config.DialContext(attemptCtx, network, address)
		elapsed := config.Now().Sub(started)
		cancel()
		if dialErr != nil {
			result.recordFailure(attempt, classifyProvinceLatencyError(dialErr))
			continue
		}
		if conn != nil {
			_ = conn.Close()
		}
		if elapsed < 0 {
			elapsed = 0
		}
		result.recordSuccess(attempt, elapsed)
	}
	result.finish()
	return result, nil
}

// RunProvinceLatency runs the caller-selected targets with bounded
// concurrency and preserves their input order in the result.
func RunProvinceLatency(ctx context.Context, targets []model.ProvinceLatencyTarget, config ProvinceLatencyConfig) []ProvinceLatencyResult {
	if ctx == nil {
		ctx = context.Background()
	}
	config = config.withDefaults()
	results := make([]ProvinceLatencyResult, len(targets))
	if len(targets) == 0 {
		return results
	}

	workers := config.Concurrency
	if workers > len(targets) {
		workers = len(targets)
	}
	jobs := make(chan int, len(targets))
	for index := range targets {
		jobs <- index
	}
	close(jobs)

	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer workersWG.Done()
			for index := range jobs {
				result, err := RunProvinceLatencyTarget(ctx, targets[index], config)
				if err != nil {
					result.InvalidInput = err.Error()
					for attempt := 1; attempt <= config.Attempts; attempt++ {
						result.recordFailure(attempt, ProvinceLatencyErrorUnknown)
					}
					result.finish()
				}
				results[index] = result
			}
		}()
	}
	workersWG.Wait()
	return results
}

func newProvinceLatencyResult(target model.ProvinceLatencyTarget, config ProvinceLatencyConfig) ProvinceLatencyResult {
	return ProvinceLatencyResult{
		Target:      target,
		Port:        config.Port,
		Attempts:    config.Attempts,
		Samples:     make([]ProvinceLatencySample, 0, config.Attempts),
		ErrorCounts: make(map[string]int),
	}
}

func provinceLatencyNetwork(ipVersion string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(ipVersion)) {
	case "ipv4", "4":
		return "tcp4", nil
	case "ipv6", "6":
		return "tcp6", nil
	default:
		return "", fmt.Errorf("province latency IP version %q is invalid", ipVersion)
	}
}

func (result *ProvinceLatencyResult) recordSuccess(attempt int, duration time.Duration) {
	result.Successful++
	result.Samples = append(result.Samples, ProvinceLatencySample{
		Attempt:  attempt,
		Duration: duration,
		Success:  true,
	})
}

func (result *ProvinceLatencyResult) recordFailure(attempt int, errorClass string) {
	result.Failed++
	result.ErrorCounts[errorClass]++
	result.Samples = append(result.Samples, ProvinceLatencySample{
		Attempt:    attempt,
		ErrorClass: errorClass,
	})
}

func (result *ProvinceLatencyResult) finish() {
	if result.Attempts > 0 {
		result.SuccessRate = float64(result.Successful) * 100 / float64(result.Attempts)
		result.LossPercent = float64(result.Failed) * 100 / float64(result.Attempts)
	}
	latencies := make([]time.Duration, 0, result.Successful)
	for _, sample := range result.Samples {
		if sample.Success {
			latencies = append(latencies, sample.Duration)
		}
	}
	if len(latencies) == 0 {
		return
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	result.Min = latencies[0]
	result.Max = latencies[len(latencies)-1]
	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}
	result.Mean = total / time.Duration(len(latencies))
	result.P50 = provinceLatencyPercentile(latencies, 0.50)
	result.P95 = provinceLatencyPercentile(latencies, 0.95)
}

func provinceLatencyPercentile(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	position := quantile * float64(len(values)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return values[lower]
	}
	weight := position - float64(lower)
	return time.Duration(float64(values[lower])*(1-weight) + float64(values[upper])*weight)
}

func classifyProvinceLatencyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return ProvinceLatencyErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProvinceLatencyErrorTimeout
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return ProvinceLatencyErrorDNS
	}
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		return ProvinceLatencyErrorRefused
	}
	if errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return ProvinceLatencyErrorUnreachable
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return ProvinceLatencyErrorTimeout
	}
	return ProvinceLatencyErrorUnknown
}
