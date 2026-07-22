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

func TestNormalizeAndValidateCLIOptions(t *testing.T) {
	language, checkType, location := normalizeLegacyOptions(" EN ", " BOTH ", "bj")
	if language != "en" || checkType != "both" || location != "BJ" {
		t.Fatalf("options were not normalized: %q %q %q", language, checkType, location)
	}
	valid := func(visited map[string]bool, target string, deep, registry bool) error {
		return validateCLIOptions(nil, language, checkType, location, target, "both", 2, time.Second, 4, 80, deep, false, registry, visited)
	}
	if err := valid(nil, "", false, false); err != nil {
		t.Fatalf("valid legacy options failed: %v", err)
	}
	if err := valid(map[string]bool{"province-timeout": true}, "", false, false); err == nil {
		t.Fatal("ignored province-only option was accepted in legacy mode")
	}
	if err := valid(map[string]bool{"province-attempts": true}, "BJ,ct,ipv4,example.test", true, false); err == nil {
		t.Fatal("deep mode accepted an unused attempts option")
	}
	if err := valid(map[string]bool{"province-registry": true}, "", false, true); err != nil {
		t.Fatalf("registry mode was rejected: %v", err)
	}
}

func TestValidateCLIOptionsRejectsInvalidLegacyValues(t *testing.T) {
	for name, values := range map[string][3]string{
		"language":   {"fr", "ipv4", "GZ"},
		"check type": {"zh", "typo", "GZ"},
		"location":   {"zh", "ipv4", "XX"},
	} {
		if err := validateCLIOptions(nil, values[0], values[1], values[2], "", "both", 2, time.Second, 4, 80, false, false, false, nil); err == nil {
			t.Fatalf("invalid %s was accepted", name)
		}
	}
	if err := validateCLIOptions([]string{"extra"}, "zh", "ipv4", "GZ", "", "both", 2, time.Second, 4, 80, false, false, false, nil); err == nil {
		t.Fatal("positional argument was accepted")
	}
}

func TestValidateCLIOptionsAcceptsExplicitFalseModeFlagsAndRejectsConflictingSources(t *testing.T) {
	visited := map[string]bool{"deep": true, "json": true, "province-registry": true}
	if err := validateCLIOptions(nil, "zh", "ipv4", "GZ", "", "both", 2, time.Second, 4, 80, false, false, false, visited); err != nil {
		t.Fatalf("explicit false flags were treated as enabled: %v", err)
	}
	if err := validateCLIOptions(nil, "zh", "ipv4", "GZ", "BJ,ct,ipv4,example.test", "both", 2, time.Second, 4, 80, false, false, true, visited); err == nil {
		t.Fatal("conflicting explicit and registry targets were accepted")
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
