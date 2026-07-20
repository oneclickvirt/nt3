package model

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

//go:embed snapshot/province-routes.json
var embeddedProvinceRoutes []byte

const (
	ProvinceRouteRegistryRawURL = "https://raw.githubusercontent.com/oneclickvirt/nt3/main/model/snapshot/province-routes.json"
	ProvinceRouteRegistryCDNURL = "https://cdn.spiritlhl.net/" + ProvinceRouteRegistryRawURL
)

type ProvinceRouteRegistrySource struct {
	Name string
	URL  string
}

type ProvinceRouteRegistryLoadResult struct {
	Routes   []ProvinceRoute
	Source   string
	Fallback bool
}

func DefaultProvinceRouteRegistrySources() []ProvinceRouteRegistrySource {
	return []ProvinceRouteRegistrySource{
		{Name: "cdn", URL: ProvinceRouteRegistryCDNURL},
		{Name: "raw", URL: ProvinceRouteRegistryRawURL},
	}
}

func NormalizeProvinceRouteSnapshot(data []byte) ([]byte, error) {
	routes, err := ParseProvinceRoutes(data)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func LoadProvinceRoutes(ctx context.Context, client *http.Client, sources []ProvinceRouteRegistrySource) (ProvinceRouteRegistryLoadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	var lastErr error
	for index, source := range sources {
		data, err := fetchProvinceRouteSnapshot(ctx, client, source.URL)
		if err != nil {
			lastErr = fmt.Errorf("load %s province routes: %w", source.Name, err)
			continue
		}
		routes, err := ParseProvinceRoutes(data)
		if err != nil {
			lastErr = fmt.Errorf("validate %s province routes: %w", source.Name, err)
			continue
		}
		return ProvinceRouteRegistryLoadResult{Routes: routes, Source: source.Name, Fallback: index > 0}, nil
	}
	routes, err := ParseProvinceRoutes(embeddedProvinceRoutes)
	if err == nil {
		return ProvinceRouteRegistryLoadResult{Routes: routes, Source: "embedded", Fallback: true}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no province route registry sources configured")
	}
	return ProvinceRouteRegistryLoadResult{}, fmt.Errorf("%w; embedded fallback: %v", lastErr, err)
}

func fetchProvinceRouteSnapshot(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, errors.New("invalid registry URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "oneclickvirt-nt3/province-registry-v1")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 2<<20))
}

// ProvinceRouteSnapshotEntry is the upstream province metadata shape used by
// the updater. Runtime only keeps the smaller validated ProvinceRoute shape.
type ProvinceRouteSnapshotEntry struct {
	Code     string `json:"code"`
	Short    string `json:"short"`
	Name     string `json:"name"`
	Province int    `json:"province"`
	Zipcode  int    `json:"zipcode"`
}

func BuildProvinceRoutesFromMetadata(entries []ProvinceRouteSnapshotEntry) ([]ProvinceRoute, error) {
	routes := make([]ProvinceRoute, 0, len(entries))
	for _, entry := range entries {
		if entry.Province >= 70 {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(entry.Code))
		lower := strings.ToLower(code)
		routes = append(routes, ProvinceRoute{
			Code: code, Name: strings.TrimSpace(entry.Name), Province: entry.Province, Short: strings.TrimSpace(entry.Short),
			Targets: []ProvinceCarrierTarget{
				{Carrier: "ct", IPv4: lower + "-ct-v4.ip.zstaticcdn.com", IPv6: lower + "-ct-v6.ip.zstaticcdn.com"},
				{Carrier: "cu", IPv4: lower + "-cu-v4.ip.zstaticcdn.com", IPv6: lower + "-cu-v6.ip.zstaticcdn.com"},
				{Carrier: "cm", IPv4: lower + "-cm-v4.ip.zstaticcdn.com", IPv6: lower + "-cm-v6.ip.zstaticcdn.com"},
			},
		})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Province < routes[j].Province })
	return ValidateProvinceRoutes(routes)
}
