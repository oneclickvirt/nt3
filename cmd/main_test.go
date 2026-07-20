package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oneclickvirt/nt3/model"
	"github.com/oneclickvirt/nt3/nt"
)

func TestParseProvinceTargets(t *testing.T) {
	targets, err := parseProvinceTargets("BJ,ct,ipv4,198.51.100.1;SH,cu,ipv6,2001:db8::1;BJ,ct,ipv4,198.51.100.1", "both")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[1].Host != "2001:db8::1" || targets[1].IPVersion != "ipv6" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestHelpAndVersionPrecedeProvinceValidation(t *testing.T) {
	for _, test := range []struct {
		help, version, jsonOutput, deep bool
		want                            cliAction
	}{
		{help: true, jsonOutput: true, want: cliHelp},
		{version: true, deep: true, want: cliVersion},
	} {
		action, err := selectCLIAction(test.help, test.version, test.jsonOutput, test.deep, "")
		if err != nil || action != test.want {
			t.Fatalf("selectCLIAction(%+v) = %v, %v", test, action, err)
		}
	}
}

func TestSelectCLIActionAcceptsProvinceRegistry(t *testing.T) {
	action, err := selectCLIAction(false, false, true, false, "", true)
	if err != nil || action != cliProvince {
		t.Fatalf("province registry action = %v, %v", action, err)
	}
}

func TestParseProvinceTargetsExpandsDefaultBoth(t *testing.T) {
	targets, err := parseProvinceTargets("BJ,ct,,localhost", "both")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].IPVersion != "ipv4" || targets[1].IPVersion != "ipv6" {
		t.Fatalf("both target was not expanded in order: %+v", targets)
	}
}

func TestRunProvinceModeJSONUsesProbeParameters(t *testing.T) {
	var output bytes.Buffer
	err := runProvinceMode(context.Background(), &output, "BJ,ct,ipv4,localhost", "both", 1, time.Millisecond, 1, 1, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Attempts int `json:"attempts"`
		Port     int `json:"port"`
	}
	if err := json.Unmarshal(output.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, output.String())
	}
	if len(results) != 1 || results[0].Attempts != 1 || results[0].Port != 1 {
		t.Fatalf("probe parameters not reflected: %+v", results)
	}
}

func TestRunProvinceModeRejectsInvalidRegistryIPVersion(t *testing.T) {
	err := runProvinceMode(context.Background(), &bytes.Buffer{}, "", "typo", 1, time.Second, 1, 80, false, true, true)
	if err == nil {
		t.Fatal("expected invalid registry IP version to fail")
	}
}

func TestRunDeepProvinceTargetsAppliesPerTargetTimeout(t *testing.T) {
	targets := []model.ProvinceLatencyTarget{{ProvinceCode: "BJ", Carrier: "ct", IPVersion: "ipv4", Host: "fixture"}}
	results := runDeepProvinceTargets(context.Background(), targets, 1, 5*time.Millisecond, func(ctx context.Context, _ model.ProvinceLatencyTarget) ([]nt.ProvinceRouteHop, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if len(results) != 1 || results[0].Status != nt.ProvinceRouteStatusCanceled || results[0].Duration > 100*time.Millisecond {
		t.Fatalf("deep timeout not applied: %+v", results)
	}
}
