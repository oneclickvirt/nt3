package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAndBuildProvinceLatencyTargets(t *testing.T) {
	routes := validProvinceRoutes()
	data, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProvinceRoutes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(BuildProvinceLatencyTargets(parsed, "both")); got != 31*3*2 {
		t.Fatalf("got %d targets", got)
	}
	if got := len(BuildProvinceLatencyTargets(parsed, "ipv4")); got != 31*3 {
		t.Fatalf("got %d IPv4 targets", got)
	}
}

func TestParseProvinceRoutesNormalizesCurrentSchema(t *testing.T) {
	routes := validProvinceRoutes()
	routes[0].Code = " aa "
	routes[0].Name = " Province-01 "
	routes[0].Short = " 01 "
	routes[0].Targets[0] = ProvinceCarrierTarget{
		Carrier: " CT ",
		IPv4:    "AA-CT-V4.EXAMPLE.",
		IPv6:    "AA-CT-V6.EXAMPLE.",
	}
	data, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProvinceRoutes(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed[0].Code != "AA" || parsed[0].Name != "Province-01" || parsed[0].Targets[0].Carrier != "ct" {
		t.Fatalf("route was not normalized: %+v", parsed[0])
	}
	if parsed[0].Targets[0].IPv4 != "aa-ct-v4.example" {
		t.Fatalf("host was not normalized: %q", parsed[0].Targets[0].IPv4)
	}
}

func TestParseProvinceRoutesRejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]ProvinceRoute) []ProvinceRoute
		want   string
	}{
		{name: "province count", mutate: func(routes []ProvinceRoute) []ProvinceRoute { return routes[:30] }, want: "want 31"},
		{name: "duplicate carrier", mutate: func(routes []ProvinceRoute) []ProvinceRoute {
			routes[0].Targets[1].Carrier = "ct"
			return routes
		}, want: "duplicate carrier"},
		{name: "unknown carrier", mutate: func(routes []ProvinceRoute) []ProvinceRoute {
			routes[0].Targets[0].Carrier = "other"
			return routes
		}, want: "invalid carrier"},
		{name: "invalid host", mutate: func(routes []ProvinceRoute) []ProvinceRoute {
			routes[0].Targets[0].IPv4 = "https://invalid.example"
			return routes
		}, want: "not a valid hostname"},
		{name: "wrong literal family", mutate: func(routes []ProvinceRoute) []ProvinceRoute {
			routes[0].Targets[0].IPv6 = "192.0.2.1"
			return routes
		}, want: "IPv4 literal"},
		{name: "duplicate province", mutate: func(routes []ProvinceRoute) []ProvinceRoute {
			routes[1].Province = routes[0].Province
			return routes
		}, want: "duplicate province number"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.mutate(validProvinceRoutes()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseProvinceRoutes(data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseProvinceRoutes error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseProvinceRoutesRejectsUnknownAndTrailingFields(t *testing.T) {
	data, err := json.Marshal(validProvinceRoutes())
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := strings.Replace(string(data), `"code":"AA"`, `"code":"AA","extra":true`, 1)
	if _, err := ParseProvinceRoutes([]byte(withUnknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := ParseProvinceRoutes(append(data, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing value error = %v", err)
	}
}

func validProvinceRoutes() []ProvinceRoute {
	codes := []string{
		"AA", "AB", "AC", "AD", "AE", "AF", "AG", "AH", "AI", "AJ",
		"AK", "AL", "AM", "AN", "AO", "AP", "AQ", "AR", "AS", "AT",
		"AU", "AV", "AW", "AX", "AY", "AZ", "BA", "BB", "BC", "BD", "BE",
	}
	routes := make([]ProvinceRoute, len(codes))
	for index, code := range codes {
		prefix := strings.ToLower(code)
		routes[index] = ProvinceRoute{
			Code:     code,
			Name:     "Province-" + code,
			Province: index + 1,
			Short:    code,
			Targets: []ProvinceCarrierTarget{
				{Carrier: "ct", IPv4: prefix + "-ct-v4.example", IPv6: prefix + "-ct-v6.example"},
				{Carrier: "cu", IPv4: prefix + "-cu-v4.example", IPv6: prefix + "-cu-v6.example"},
				{Carrier: "cm", IPv4: prefix + "-cm-v4.example", IPv6: prefix + "-cm-v6.example"},
			},
		}
	}
	return routes
}
