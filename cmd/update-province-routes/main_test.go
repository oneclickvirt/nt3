package main

import "testing"

func TestUpdateConfigDefaultsAreValid(t *testing.T) {
	config := updateConfig{Source: defaultSourceURL, Output: "snapshot.json"}
	if config.Source == "" || config.Output == "" {
		t.Fatal("invalid updater defaults")
	}
}
