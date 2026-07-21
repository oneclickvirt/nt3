package nt

import (
	"context"
	"errors"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oneclickvirt/nt3/model"
)

func TestRunProvinceLatencyTargetCalculatesMetrics(t *testing.T) {
	base := time.Unix(0, 0)
	times := []time.Time{
		base, base.Add(time.Millisecond),
		base, base.Add(2 * time.Millisecond),
		base, base.Add(3 * time.Millisecond),
		base, base.Add(4 * time.Millisecond),
	}
	var clockIndex int
	result, err := RunProvinceLatencyTarget(context.Background(), provinceTarget("ipv4", "latency.test"), ProvinceLatencyConfig{
		Attempts: 4,
		Timeout:  time.Second,
		Port:     443,
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp4" || address != "latency.test:443" {
				t.Fatalf("unexpected dial arguments: %s %s", network, address)
			}
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
		Now: func() time.Time {
			current := times[clockIndex]
			clockIndex++
			return current
		},
	})
	if err != nil {
		t.Fatalf("RunProvinceLatencyTarget returned error: %v", err)
	}
	if result.Successful != 4 || result.Failed != 0 || result.SuccessRate != 100 || result.LossPercent != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if result.Min != time.Millisecond || result.Mean != 2500*time.Microsecond || result.Max != 4*time.Millisecond {
		t.Errorf("unexpected min/mean/max: %v/%v/%v", result.Min, result.Mean, result.Max)
	}
	if result.P50 != 2500*time.Microsecond || result.P95 != 3850*time.Microsecond {
		t.Errorf("unexpected percentiles: p50=%v p95=%v", result.P50, result.P95)
	}
}

func TestProvinceLatencyPercentileRoundsToNearestNanosecond(t *testing.T) {
	values := []time.Duration{0, time.Nanosecond}
	if got := provinceLatencyPercentile(values, 0.50); got != time.Nanosecond {
		t.Fatalf("p50 = %s, want %s", got, time.Nanosecond)
	}
}

func TestRunProvinceLatencyTargetClassifiesFailures(t *testing.T) {
	errorsByAttempt := []error{
		&net.DNSError{Err: "no such host", Name: "missing.test"},
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		provinceTimeoutError{},
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH},
	}
	var index int
	result, err := RunProvinceLatencyTarget(context.Background(), provinceTarget("ipv6", "latency.test"), ProvinceLatencyConfig{
		Attempts: 4,
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp6" || address != "latency.test:80" {
				t.Fatalf("unexpected dial arguments: %s %s", network, address)
			}
			err := errorsByAttempt[index]
			index++
			return nil, err
		},
	})
	if err != nil {
		t.Fatalf("RunProvinceLatencyTarget returned error: %v", err)
	}
	want := map[string]int{
		ProvinceLatencyErrorDNS:         1,
		ProvinceLatencyErrorRefused:     1,
		ProvinceLatencyErrorTimeout:     1,
		ProvinceLatencyErrorUnreachable: 1,
	}
	if result.Successful != 0 || result.Failed != 4 || result.SuccessRate != 0 || result.LossPercent != 100 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	for class, count := range want {
		if result.ErrorCounts[class] != count {
			t.Errorf("error class %q: got %d, want %d", class, result.ErrorCounts[class], count)
		}
	}
}

func TestRunProvinceLatencyTargetHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var dialed bool
	result, err := RunProvinceLatencyTarget(ctx, provinceTarget("ipv4", "latency.test"), ProvinceLatencyConfig{
		Attempts: 3,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, errors.New("unexpected dial")
		},
	})
	if err != nil {
		t.Fatalf("RunProvinceLatencyTarget returned error: %v", err)
	}
	if dialed {
		t.Fatal("dialer called after context cancellation")
	}
	if result.Failed != 3 || result.ErrorCounts[ProvinceLatencyErrorCanceled] != 3 || result.LossPercent != 100 {
		t.Fatalf("unexpected cancellation result: %+v", result)
	}
}

func TestRunProvinceLatencyPreservesOrderAndBoundsConcurrency(t *testing.T) {
	targets := []model.ProvinceLatencyTarget{
		provinceTarget("ipv4", "one.test"),
		provinceTarget("ipv6", "two.test"),
		provinceTarget("ipv4", "three.test"),
	}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	results := RunProvinceLatency(context.Background(), targets, ProvinceLatencyConfig{
		Attempts:    1,
		Concurrency: 2,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			client, server := net.Pipe()
			_ = server.Close()
			mu.Lock()
			inFlight--
			mu.Unlock()
			return client, nil
		},
	})
	if maxInFlight > 2 {
		t.Fatalf("observed concurrency %d, want at most 2", maxInFlight)
	}
	for index, target := range targets {
		if results[index].Target != target || results[index].Successful != 1 {
			t.Errorf("result %d does not match target: %+v", index, results[index])
		}
	}
}

func TestRunProvinceLatencyRecordsInvalidFixture(t *testing.T) {
	targets := []model.ProvinceLatencyTarget{
		provinceTarget("both", "invalid.test"),
		provinceTarget("ipv4", ""),
	}
	results := RunProvinceLatency(context.Background(), targets, ProvinceLatencyConfig{Attempts: 2})
	for index, result := range results {
		if result.InvalidInput == "" || result.Failed != 2 || result.ErrorCounts[ProvinceLatencyErrorUnknown] != 2 {
			t.Errorf("invalid fixture %d was not represented: %+v", index, result)
		}
	}
}

func TestProvinceLatencyProfiles(t *testing.T) {
	standard := StandardProvinceLatencyConfig()
	deep := DeepProvinceLatencyConfig()
	if standard.Attempts >= deep.Attempts || standard.Timeout >= deep.Timeout {
		t.Fatalf("deep profile must exceed standard sampling: standard=%+v deep=%+v", standard, deep)
	}
	if got := RunProvinceLatency(context.Background(), nil, ProvinceLatencyConfig{}); len(got) != 0 {
		t.Fatalf("empty target selection should not run probes: %+v", got)
	}
}

func provinceTarget(ipVersion, host string) model.ProvinceLatencyTarget {
	return model.ProvinceLatencyTarget{
		ProvinceCode: "BJ",
		ProvinceName: "Beijing",
		Carrier:      "ct",
		IPVersion:    ipVersion,
		Host:         host,
	}
}

type provinceTimeoutError struct{}

func (provinceTimeoutError) Error() string   { return "timed out" }
func (provinceTimeoutError) Timeout() bool   { return true }
func (provinceTimeoutError) Temporary() bool { return true }
