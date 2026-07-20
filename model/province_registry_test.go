package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
