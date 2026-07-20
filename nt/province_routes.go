package nt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oneclickvirt/nt3/model"
)

const (
	ProvinceRouteStatusOK          = "ok"
	ProvinceRouteStatusError       = "error"
	ProvinceRouteStatusCanceled    = "canceled"
	ProvinceRouteStatusUnavailable = "unavailable"
	provinceRouteStatusPending     = "pending"
)

// ProvinceRouteHop is one structured hop returned by an injected tracer.
// Address may be empty for a timed-out hop while Hop remains populated.
type ProvinceRouteHop struct {
	Hop      int           `json:"hop"`
	Address  string        `json:"address,omitempty"`
	RTT      time.Duration `json:"rtt,omitempty"`
	ASN      string        `json:"asn,omitempty"`
	Location string        `json:"location,omitempty"`
}

// ProvinceRouteTracer performs one detailed route trace. Implementations must
// observe ctx so a deep run can honor cancellation and its global deadline.
type ProvinceRouteTracer func(context.Context, model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error)

// DetailedProvinceRouteConfig keeps detailed province tracing opt-in. The
// standard profile leaves Deep false and therefore never invokes Tracer.
type DetailedProvinceRouteConfig struct {
	Deep        bool
	IPVersion   string
	Concurrency int
	Tracer      ProvinceRouteTracer
}

// DetailedProvinceRouteResult is the structured outcome for one
// province/carrier/IP-version route task.
type DetailedProvinceRouteResult struct {
	Target   model.ProvinceLatencyTarget `json:"target"`
	Status   string                      `json:"status"`
	Duration time.Duration               `json:"duration"`
	Hops     []ProvinceRouteHop          `json:"hops"`
	Error    string                      `json:"error,omitempty"`
}

func StandardDetailedProvinceRouteConfig() DetailedProvinceRouteConfig {
	return DetailedProvinceRouteConfig{
		Deep:        false,
		IPVersion:   "ipv4",
		Concurrency: 3,
	}
}

func DeepDetailedProvinceRouteConfig(tracer ProvinceRouteTracer) DetailedProvinceRouteConfig {
	config := StandardDetailedProvinceRouteConfig()
	config.Deep = true
	config.Tracer = tracer
	return config
}

// RunDetailedProvinceRoutes expands all validated province/carrier targets
// only for a deep run. Per-target trace and cancellation errors are retained
// in results; the returned error is reserved for invalid configuration/data.
func RunDetailedProvinceRoutes(ctx context.Context, routes []model.ProvinceRoute, config DetailedProvinceRouteConfig) ([]DetailedProvinceRouteResult, error) {
	if !config.Deep {
		return []DetailedProvinceRouteResult{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if config.Tracer == nil {
		return nil, errors.New("detailed province route tracer is required in deep mode")
	}
	if config.Concurrency <= 0 {
		config.Concurrency = StandardDetailedProvinceRouteConfig().Concurrency
	}
	config.IPVersion = strings.ToLower(strings.TrimSpace(config.IPVersion))
	if config.IPVersion == "" {
		config.IPVersion = "ipv4"
	}
	if config.IPVersion != "ipv4" && config.IPVersion != "ipv6" && config.IPVersion != "both" {
		return nil, fmt.Errorf("detailed province route IP version %q is invalid", config.IPVersion)
	}

	validated, err := model.ValidateProvinceRoutes(routes)
	if err != nil {
		return nil, err
	}
	targets := model.BuildProvinceLatencyTargets(validated, config.IPVersion)
	results := make([]DetailedProvinceRouteResult, len(targets))
	for index, target := range targets {
		results[index] = DetailedProvinceRouteResult{
			Target: target,
			Status: provinceRouteStatusPending,
			Hops:   []ProvinceRouteHop{},
		}
	}

	workers := config.Concurrency
	if workers > len(targets) {
		workers = len(targets)
	}
	jobs := make(chan int)
	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer workersWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, open := <-jobs:
					if !open {
						return
					}
					if ctx.Err() != nil {
						return
					}
					started := time.Now()
					hops, traceErr := config.Tracer(ctx, targets[index])
					result := DetailedProvinceRouteResult{
						Target:   targets[index],
						Duration: time.Since(started),
						Hops:     append([]ProvinceRouteHop{}, hops...),
					}
					switch {
					case traceErr == nil && len(hops) > 0:
						result.Status = ProvinceRouteStatusOK
					case traceErr == nil:
						result.Status = ProvinceRouteStatusUnavailable
						result.Error = "tracer returned no hops"
					case errors.Is(traceErr, context.Canceled),
						errors.Is(traceErr, context.DeadlineExceeded) && ctx.Err() != nil:
						result.Status = ProvinceRouteStatusCanceled
						result.Error = traceErr.Error()
					default:
						result.Status = ProvinceRouteStatusError
						result.Error = traceErr.Error()
					}
					results[index] = result
				}
			}
		}()
	}

scheduleLoop:
	for index := range targets {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break scheduleLoop
		}
	}
	close(jobs)
	workersWG.Wait()

	for index := range results {
		if results[index].Status != provinceRouteStatusPending {
			continue
		}
		if err := ctx.Err(); err != nil {
			results[index].Status = ProvinceRouteStatusCanceled
			results[index].Error = err.Error()
			continue
		}
		results[index].Status = ProvinceRouteStatusUnavailable
		results[index].Error = "route task was not scheduled"
	}
	return results, nil
}
