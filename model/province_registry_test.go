package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildProvinceRoutesFromMetadata(t *testing.T) {
	codes := []string{"BJ", "TJ", "HE", "SX", "NM", "LN", "JL", "HL", "SH", "JS", "ZJ", "AH", "FJ", "JX", "SD", "HA", "HB", "HN", "GD", "GX", "HI", "CQ", "SC", "GZ", "YN", "XZ", "SN", "GS", "QH", "NX", "XJ"}
	entries := make([]ProvinceRouteSnapshotEntry, ProvinceRouteCount+1)
	for i := range entries {
		entries[i] = ProvinceRouteSnapshotEntry{Code: "P" + string(rune('A'+i%26)), Name: "Province", Short: "P", Province: i + 1}
	}
	entries[len(entries)-1].Province = 71
	for i := range entries[:ProvinceRouteCount] {
		entries[i].Code = codes[i]
		entries[i].Name = "Province " + codes[i]
		entries[i].Short = codes[i][:1]
	}
	routes, err := BuildProvinceRoutesFromMetadata(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != ProvinceRouteCount || routes[0].Targets[0].IPv4 != "bj-ct-v4.ip.zstaticcdn.com" {
		t.Fatalf("unexpected routes: len=%d first=%+v", len(routes), routes[0])
	}
}

func TestLoadProvinceRoutesRejectsBadManifestAndUsesNextSource(t *testing.T) {
	data, err := json.Marshal(validProvinceRoutes())
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	manifest := ProvinceRouteManifest{Schema: ProvinceRouteRegistrySchema, File: "province-routes.json", Count: ProvinceRouteCount, SHA256: hex.EncodeToString(hash[:]), GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	manifestData, _ := json.Marshal(manifest)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cdn-manifest":
			bad := manifest
			bad.SHA256 = strings.Repeat("0", 64)
			_ = json.NewEncoder(writer).Encode(bad)
		case "/raw-manifest":
			_, _ = writer.Write(manifestData)
		default:
			_, _ = writer.Write(data)
		}
	}))
	defer server.Close()
	loaded, err := LoadProvinceRoutes(context.Background(), server.Client(), []ProvinceRouteRegistrySource{
		{Name: "cdn", URL: server.URL + "/cdn-data", ManifestURL: server.URL + "/cdn-manifest"},
		{Name: "raw", URL: server.URL + "/raw-data", ManifestURL: server.URL + "/raw-manifest"},
	})
	if err != nil || loaded.Source != "raw" || !loaded.Fallback {
		t.Fatalf("unexpected manifest fallback: %+v, %v", loaded, err)
	}
	if loaded.Metadata.Schema != ProvinceRouteRegistrySchema || loaded.Metadata.Count != ProvinceRouteCount || loaded.Metadata.SHA256 != manifest.SHA256 {
		t.Fatalf("manifest metadata missing: %+v", loaded.Metadata)
	}
}

func TestLoadProvinceRoutesFallsBackToRawAndEmbedded(t *testing.T) {
	valid := validProvinceRoutes()
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/bad" {
			http.Error(writer, "bad", http.StatusBadGateway)
			return
		}
		_, _ = writer.Write(data)
	}))
	defer server.Close()
	loaded, err := LoadProvinceRoutes(context.Background(), server.Client(), []ProvinceRouteRegistrySource{
		{Name: "cdn", URL: server.URL + "/bad"}, {Name: "raw", URL: server.URL + "/good"},
	})
	if err != nil || loaded.Source != "raw" || !loaded.Fallback || len(loaded.Routes) != ProvinceRouteCount {
		t.Fatalf("unexpected load: %+v, %v", loaded, err)
	}
}

func TestLoadProvinceRoutesReturnsEmbeddedMetadata(t *testing.T) {
	loaded, err := LoadProvinceRoutes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source != "embedded" || !loaded.Fallback || loaded.Metadata.Schema != ProvinceRouteRegistrySchema || loaded.Metadata.Count != len(loaded.Routes) || loaded.Metadata.GeneratedAt == "" || len(loaded.Metadata.SHA256) != 64 {
		t.Fatalf("unexpected embedded metadata: %+v", loaded)
	}
}
