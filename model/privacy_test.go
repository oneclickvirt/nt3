package model

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type privacyRoundTripper func(*http.Request) (*http.Response, error)

func (fn privacyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestFetchProvinceRouteSnapshotDoesNotExposeSourceURL(t *testing.T) {
	client := &http.Client{Transport: privacyRoundTripper(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("dial " + request.URL.String())
	})}
	_, err := fetchProvinceRouteSnapshot(context.Background(), client, "https://private.example/routes?token=secret")
	if err == nil {
		t.Fatal("expected request failure")
	}
	for _, forbidden := range []string{"private.example", "token=", "secret"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error leaked %q: %v", forbidden, err)
		}
	}
}
