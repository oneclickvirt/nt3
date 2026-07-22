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
	Metadata RegistryMetadata
}

type RegistryMetadata struct {
	Schema      string `json:"schema"`
	Count       int    `json:"count"`
	SHA256      string `json:"sha256"`
	GeneratedAt string `json:"generated_at,omitempty"`
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
		data, metadata, err := loadProvinceRouteSnapshot(ctx, client, source)
		if err != nil {
			lastErr = fmt.Errorf("load %s province routes: %w", provinceRegistrySourceLabel(source.Name), err)
			continue
		}
		routes, err := ParseProvinceRoutes(data)
		if err != nil {
			lastErr = fmt.Errorf("validate %s province routes: %w", provinceRegistrySourceLabel(source.Name), err)
			continue
		}
		if metadata.Count == 0 {
			metadata = provinceRouteRegistryMetadata(data, routes)
		}
		return ProvinceRouteRegistryLoadResult{Routes: routes, Source: source.Name, Fallback: index > 0, Metadata: metadata}, nil
	}
	metadata, err := validateProvinceRouteManifest(embeddedProvinceRouteManifest, embeddedProvinceRoutes)
	if err != nil {
		return ProvinceRouteRegistryLoadResult{}, fmt.Errorf("validate embedded province route manifest: %w", err)
	}
	routes, err := ParseProvinceRoutes(embeddedProvinceRoutes)
	if err == nil {
		return ProvinceRouteRegistryLoadResult{Routes: routes, Source: "embedded", Fallback: true, Metadata: metadata}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no province route registry sources configured")
	}
	return ProvinceRouteRegistryLoadResult{}, fmt.Errorf("%w; embedded fallback: %v", lastErr, err)
}

func loadProvinceRouteSnapshot(ctx context.Context, client *http.Client, source ProvinceRouteRegistrySource) ([]byte, RegistryMetadata, error) {
	if source.ManifestURL == "" {
		data, err := fetchProvinceRouteSnapshot(ctx, client, source.URL)
		return data, RegistryMetadata{}, err
	}
	manifestData, err := fetchProvinceRouteSnapshot(ctx, client, source.ManifestURL)
	if err != nil {
		return nil, RegistryMetadata{}, fmt.Errorf("load manifest: %w", err)
	}
	data, err := fetchProvinceRouteSnapshot(ctx, client, source.URL)
	if err != nil {
		return nil, RegistryMetadata{}, err
	}
	metadata, err := validateProvinceRouteManifest(manifestData, data)
	if err != nil {
		return nil, RegistryMetadata{}, err
	}
	return data, metadata, nil
}

func validateProvinceRouteManifest(data, snapshot []byte) (RegistryMetadata, error) {
	var manifest ProvinceRouteManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return RegistryMetadata{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return RegistryMetadata{}, err
	}
	if manifest.Schema != ProvinceRouteRegistrySchema || manifest.File != "province-routes.json" || manifest.Count < 1 {
		return RegistryMetadata{}, errors.New("manifest schema, file, or count is invalid")
	}
	if _, err := time.Parse(time.RFC3339, manifest.GeneratedAt); err != nil {
		return RegistryMetadata{}, fmt.Errorf("manifest generated_at is invalid: %w", err)
	}
	hash := sha256.Sum256(snapshot)
	if !strings.EqualFold(manifest.SHA256, hex.EncodeToString(hash[:])) {
		return RegistryMetadata{}, errors.New("manifest SHA-256 does not match snapshot")
	}
	routes, err := ParseProvinceRoutes(snapshot)
	if err != nil {
		return RegistryMetadata{}, fmt.Errorf("manifest snapshot validation failed: %w", err)
	}
	if len(routes) != manifest.Count {
		return RegistryMetadata{}, fmt.Errorf("manifest count %d does not match snapshot count %d", manifest.Count, len(routes))
	}
	return RegistryMetadata{Schema: manifest.Schema, Count: manifest.Count, SHA256: strings.ToLower(manifest.SHA256), GeneratedAt: manifest.GeneratedAt}, nil
}

func provinceRouteRegistryMetadata(snapshot []byte, routes []ProvinceRoute) RegistryMetadata {
	hash := sha256.Sum256(snapshot)
	return RegistryMetadata{Schema: ProvinceRouteRegistrySchema, Count: len(routes), SHA256: hex.EncodeToString(hash[:])}
}

func fetchProvinceRouteSnapshot(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, errors.New("invalid registry URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errors.New("create registry request failed")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "oneclickvirt-nt3/province-registry-v1")
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, errors.New("registry request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, errors.New("registry response read failed")
	}
	return data, nil
}

func provinceRegistrySourceLabel(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cdn", "raw":
		return strings.ToLower(strings.TrimSpace(name))
	default:
		return "remote"
	}
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
