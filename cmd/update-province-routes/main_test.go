package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestUpdateConfigDefaultsAreValid(t *testing.T) {
	config := updateConfig{Source: defaultSourceURL, Output: "snapshot.json"}
	if config.Source == "" || config.Output == "" {
		t.Fatal("invalid updater defaults")
	}
}

func TestUpdateSnapshotRedactsSourceURLFromFetchErrors(t *testing.T) {
	source := "https://private.example.invalid/provinces?key=do-not-print"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial " + source)
	})}
	err := updateSnapshot(context.Background(), client, updateConfig{
		Source: source, Output: t.TempDir() + "/routes.json", Manifest: t.TempDir() + "/manifest.json", Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("fetch failure unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), source) || strings.Contains(err.Error(), "do-not-print") {
		t.Fatalf("fetch error exposed source URL: %q", err)
	}
}
