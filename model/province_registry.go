package model

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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

//go:embed snapshot/manifest.json
var embeddedProvinceRouteManifest []byte

const (
	ProvinceRouteRegistrySchema = "nt3.province-routes/v1"
	ProvinceRouteRegistryRawURL = "https://raw.githubusercontent.com/oneclickvirt/nt3/main/model/snapshot/province-routes.json"
	ProvinceRouteManifestRawURL = "https://raw.githubusercontent.com/oneclickvirt/nt3/main/model/snapshot/manifest.json"
	ProvinceRouteRegistryCDNURL = "https://cdn.spiritlhl.net/" + ProvinceRouteRegistryRawURL
	ProvinceRouteManifestCDNURL = "https://cdn.spiritlhl.net/" + ProvinceRouteManifestRawURL
)

type ProvinceRouteRegistrySource struct {
	Name        string
	URL         string
	ManifestURL string
}

type ProvinceRouteRegistryLoadResult struct {
	Routes   []ProvinceRoute
	Source   string
	Fallback bool
}

func DefaultProvinceRouteRegistrySources() []ProvinceRouteRegistrySource {
	return []ProvinceRouteRegistrySource{
		{Name: "cdn", URL: ProvinceRouteRegistryCDNURL, ManifestURL: ProvinceRouteManifestCDNURL},
		{Name: "raw", URL: ProvinceRouteRegistryRawURL, ManifestURL: ProvinceRouteManifestRawURL},
	}
}

type ProvinceRouteManifest struct {
	Schema      string `json:"schema"`
	File        string `json:"file"`
	Count       int    `json:"count"`
	SHA256      string `json:"sha256"`
	GeneratedAt string `json:"generated_at"`
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
		data, err := loadProvinceRouteSnapshot(ctx, client, source)
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
	if err := validateProvinceRouteManifest(embeddedProvinceRouteManifest, embeddedProvinceRoutes); err != nil {
		return ProvinceRouteRegistryLoadResult{}, fmt.Errorf("validate embedded province route manifest: %w", err)
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

func loadProvinceRouteSnapshot(ctx context.Context, client *http.Client, source ProvinceRouteRegistrySource) ([]byte, error) {
	if source.ManifestURL == "" {
		return fetchProvinceRouteSnapshot(ctx, client, source.URL)
	}
	manifestData, err := fetchProvinceRouteSnapshot(ctx, client, source.ManifestURL)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	data, err := fetchProvinceRouteSnapshot(ctx, client, source.URL)
	if err != nil {
		return nil, err
	}
	if err := validateProvinceRouteManifest(manifestData, data); err != nil {
		return nil, err
	}
	return data, nil
}

func validateProvinceRouteManifest(data, snapshot []byte) error {
	var manifest ProvinceRouteManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if manifest.Schema != ProvinceRouteRegistrySchema || manifest.File != "province-routes.json" || manifest.Count < 1 {
		return errors.New("manifest schema, file, or count is invalid")
	}
	if _, err := time.Parse(time.RFC3339, manifest.GeneratedAt); err != nil {
		return fmt.Errorf("manifest generated_at is invalid: %w", err)
	}
	hash := sha256.Sum256(snapshot)
	if !strings.EqualFold(manifest.SHA256, hex.EncodeToString(hash[:])) {
		return errors.New("manifest SHA-256 does not match snapshot")
	}
	routes, err := ParseProvinceRoutes(snapshot)
	if err != nil {
		return fmt.Errorf("manifest snapshot validation failed: %w", err)
	}
	if len(routes) != manifest.Count {
		return fmt.Errorf("manifest count %d does not match snapshot count %d", manifest.Count, len(routes))
	}
	return nil
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
