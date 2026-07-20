package nt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oneclickvirt/nt3/model"
)

func TestStandardDetailedProvinceRoutesDoesNotInvokeTracer(t *testing.T) {
	config := StandardDetailedProvinceRouteConfig()
	config.Tracer = func(context.Context, model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
		panic("standard mode invoked detailed tracer")
	}
	results, err := RunDetailedProvinceRoutes(context.Background(), nil, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("standard mode returned detailed results: %+v", results)
	}
}

func TestDeepDetailedProvinceRoutesExpandsAll31Provinces(t *testing.T) {
	routes := detailedProvinceRoutesFixture()
	var mu sync.Mutex
	seenProvinces := make(map[string]int, model.ProvinceRouteCount)
	config := DeepDetailedProvinceRouteConfig(func(_ context.Context, target model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
		mu.Lock()
		seenProvinces[target.ProvinceCode]++
		mu.Unlock()
		return []ProvinceRouteHop{{Hop: 1, Address: target.Host, RTT: time.Millisecond}}, nil
	})
	config.IPVersion = "both"
	config.Concurrency = 7
	results, err := RunDetailedProvinceRoutes(context.Background(), routes, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != model.ProvinceRouteCount*3*2 {
		t.Fatalf("deep route task count = %d, want %d", len(results), model.ProvinceRouteCount*3*2)
	}
	if len(seenProvinces) != model.ProvinceRouteCount {
		t.Fatalf("deep route provinces = %d, want %d", len(seenProvinces), model.ProvinceRouteCount)
	}
	for code, count := range seenProvinces {
		if count != 6 {
			t.Errorf("province %s task count = %d, want 6", code, count)
		}
	}
	for index, result := range results {
		if result.Status != ProvinceRouteStatusOK || len(result.Hops) != 1 {
			t.Errorf("result %d = %+v", index, result)
		}
	}
}

func TestDeepDetailedProvinceRoutesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var once sync.Once
	config := DeepDetailedProvinceRouteConfig(func(ctx context.Context, _ model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return nil, ctx.Err()
	})
	config.Concurrency = 1

	done := make(chan []DetailedProvinceRouteResult, 1)
	errChan := make(chan error, 1)
	go func() {
		results, err := RunDetailedProvinceRoutes(ctx, detailedProvinceRoutesFixture(), config)
		if err != nil {
			errChan <- err
			return
		}
		done <- results
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("tracer did not start")
	}
	select {
	case err := <-errChan:
		t.Fatal(err)
	case results := <-done:
		if len(results) != model.ProvinceRouteCount*3 {
			t.Fatalf("canceled result count = %d", len(results))
		}
		for index, result := range results {
			if result.Status != ProvinceRouteStatusCanceled || !strings.Contains(result.Error, "canceled") {
				t.Errorf("canceled result %d = %+v", index, result)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("deep route runner did not return after cancellation")
	}
}

func TestDeepDetailedProvinceRoutesValidatesConfiguration(t *testing.T) {
	config := DeepDetailedProvinceRouteConfig(nil)
	if _, err := RunDetailedProvinceRoutes(context.Background(), detailedProvinceRoutesFixture(), config); err == nil {
		t.Fatal("missing tracer was accepted")
	}
	config.Tracer = func(context.Context, model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
		return nil, errors.New("fixture")
	}
	config.IPVersion = "invalid"
	if _, err := RunDetailedProvinceRoutes(context.Background(), detailedProvinceRoutesFixture(), config); err == nil {
		t.Fatal("invalid IP version was accepted")
	}
}

func TestDeepDetailedProvinceRoutesKeepsTracerTimeoutAsError(t *testing.T) {
	config := DeepDetailedProvinceRouteConfig(func(context.Context, model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
		return nil, context.DeadlineExceeded
	})
	results, err := RunDetailedProvinceRoutes(context.Background(), detailedProvinceRoutesFixture(), config)
	if err != nil {
		t.Fatal(err)
	}
	for index, result := range results {
		if result.Status != ProvinceRouteStatusError || !strings.Contains(result.Error, "deadline exceeded") {
			t.Errorf("timeout result %d = %+v", index, result)
		}
	}
}

func detailedProvinceRoutesFixture() []model.ProvinceRoute {
	codes := []string{
		"AA", "AB", "AC", "AD", "AE", "AF", "AG", "AH", "AI", "AJ",
		"AK", "AL", "AM", "AN", "AO", "AP", "AQ", "AR", "AS", "AT",
		"AU", "AV", "AW", "AX", "AY", "AZ", "BA", "BB", "BC", "BD", "BE",
	}
	routes := make([]model.ProvinceRoute, len(codes))
	for index, code := range codes {
		prefix := strings.ToLower(code)
		routes[index] = model.ProvinceRoute{
			Code:     code,
			Name:     "Province-" + code,
			Province: index + 1,
			Short:    code,
			Targets: []model.ProvinceCarrierTarget{
				{Carrier: "ct", IPv4: prefix + "-ct-v4.example", IPv6: prefix + "-ct-v6.example"},
				{Carrier: "cu", IPv4: prefix + "-cu-v4.example", IPv6: prefix + "-cu-v6.example"},
				{Carrier: "cm", IPv4: prefix + "-cm-v4.example", IPv6: prefix + "-cm-v6.example"},
			},
		}
	}
	return routes
}
