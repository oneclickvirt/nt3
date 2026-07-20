package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"
)

const ProvinceRouteCount = 31

var provinceCarriers = map[string]struct{}{
	"ct": {},
	"cu": {},
	"cm": {},
}

type ProvinceCarrierTarget struct {
	Carrier string `json:"carrier"`
	IPv4    string `json:"ipv4"`
	IPv6    string `json:"ipv6"`
}

type ProvinceRoute struct {
	Code     string                  `json:"code"`
	Name     string                  `json:"name"`
	Province int                     `json:"province"`
	Short    string                  `json:"short"`
	Targets  []ProvinceCarrierTarget `json:"targets"`
}

type ProvinceLatencyTarget struct {
	ProvinceCode string `json:"province_code"`
	ProvinceName string `json:"province_name"`
	Carrier      string `json:"carrier"`
	IPVersion    string `json:"ip_version"`
	Host         string `json:"host"`
}

func ParseProvinceRoutes(data []byte) ([]ProvinceRoute, error) {
	var routes []ProvinceRoute
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&routes); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return ValidateProvinceRoutes(routes)
}

// ValidateProvinceRoutes validates and normalizes an already decoded route
// snapshot. A deep copy is returned so validation never mutates caller data.
func ValidateProvinceRoutes(routes []ProvinceRoute) ([]ProvinceRoute, error) {
	if len(routes) != ProvinceRouteCount {
		return nil, fmt.Errorf("province route data contains %d provinces, want %d", len(routes), ProvinceRouteCount)
	}
	normalized := make([]ProvinceRoute, len(routes))
	seenCodes := make(map[string]struct{}, len(routes))
	seenNames := make(map[string]struct{}, len(routes))
	seenProvinces := make(map[int]struct{}, len(routes))
	for routeIndex, route := range routes {
		route.Code = strings.ToUpper(strings.TrimSpace(route.Code))
		route.Name = strings.TrimSpace(route.Name)
		route.Short = strings.TrimSpace(route.Short)
		if !validProvinceCode(route.Code) {
			return nil, fmt.Errorf("province route %d has invalid code %q", routeIndex, route.Code)
		}
		if route.Name == "" || route.Short == "" || route.Province <= 0 {
			return nil, fmt.Errorf("province route %q has invalid identity fields", route.Code)
		}
		if _, exists := seenCodes[route.Code]; exists {
			return nil, fmt.Errorf("province route data contains duplicate code %q", route.Code)
		}
		if _, exists := seenNames[route.Name]; exists {
			return nil, fmt.Errorf("province route data contains duplicate name %q", route.Name)
		}
		if _, exists := seenProvinces[route.Province]; exists {
			return nil, fmt.Errorf("province route data contains duplicate province number %d", route.Province)
		}
		seenCodes[route.Code] = struct{}{}
		seenNames[route.Name] = struct{}{}
		seenProvinces[route.Province] = struct{}{}

		if len(route.Targets) != len(provinceCarriers) {
			return nil, fmt.Errorf("province route %q contains %d carrier targets, want 3", route.Code, len(route.Targets))
		}
		route.Targets = append([]ProvinceCarrierTarget(nil), route.Targets...)
		seenCarriers := make(map[string]struct{}, len(route.Targets))
		for targetIndex, target := range route.Targets {
			target.Carrier = strings.ToLower(strings.TrimSpace(target.Carrier))
			if _, valid := provinceCarriers[target.Carrier]; !valid {
				return nil, fmt.Errorf("province route %q has invalid carrier %q", route.Code, target.Carrier)
			}
			if _, duplicate := seenCarriers[target.Carrier]; duplicate {
				return nil, fmt.Errorf("province route %q contains duplicate carrier %q", route.Code, target.Carrier)
			}
			seenCarriers[target.Carrier] = struct{}{}
			var err error
			target.IPv4, err = normalizeRouteHost(target.IPv4, 4)
			if err != nil {
				return nil, fmt.Errorf("province route %q carrier %q IPv4 host: %w", route.Code, target.Carrier, err)
			}
			target.IPv6, err = normalizeRouteHost(target.IPv6, 6)
			if err != nil {
				return nil, fmt.Errorf("province route %q carrier %q IPv6 host: %w", route.Code, target.Carrier, err)
			}
			route.Targets[targetIndex] = target
		}
		for carrier := range provinceCarriers {
			if _, exists := seenCarriers[carrier]; !exists {
				return nil, fmt.Errorf("province route %q is missing carrier %q", route.Code, carrier)
			}
		}
		normalized[routeIndex] = route
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Province < normalized[j].Province })
	return normalized, nil
}

func BuildProvinceLatencyTargets(routes []ProvinceRoute, ipVersion string) []ProvinceLatencyTarget {
	ipVersion = strings.ToLower(strings.TrimSpace(ipVersion))
	if ipVersion != "ipv4" && ipVersion != "ipv6" && ipVersion != "both" {
		ipVersion = "both"
	}
	result := make([]ProvinceLatencyTarget, 0, len(routes)*6)
	for _, route := range routes {
		for _, target := range route.Targets {
			if ipVersion == "ipv4" || ipVersion == "both" {
				result = append(result, ProvinceLatencyTarget{ProvinceCode: route.Code, ProvinceName: route.Name, Carrier: target.Carrier, IPVersion: "ipv4", Host: target.IPv4})
			}
			if ipVersion == "ipv6" || ipVersion == "both" {
				result = append(result, ProvinceLatencyTarget{ProvinceCode: route.Code, ProvinceName: route.Name, Carrier: target.Carrier, IPVersion: "ipv6", Host: target.IPv6})
			}
		}
	}
	return result
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("province route data contains trailing JSON value")
		}
		return err
	}
	return nil
}

func validProvinceCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	return code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z'
}

func normalizeRouteHost(host string, ipVersion int) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("is empty")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if ipVersion == 4 && !address.Is4() {
			return "", fmt.Errorf("contains an IPv6 literal")
		}
		if ipVersion == 6 && !address.Is6() {
			return "", fmt.Errorf("contains an IPv4 literal")
		}
		return address.String(), nil
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if len(host) == 0 || len(host) > 253 || strings.Contains(host, ":") {
		return "", fmt.Errorf("%q is not a valid hostname", host)
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("%q is not a valid hostname", host)
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') ||
				(character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", fmt.Errorf("%q is not a valid hostname", host)
		}
	}
	return host, nil
}
