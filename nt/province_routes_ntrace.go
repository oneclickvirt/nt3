package nt

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nxtrace/NTrace-core/ipgeo"
	"github.com/nxtrace/NTrace-core/trace"
	"github.com/oneclickvirt/nt3/model"
)

// NTraceProvinceTracer is the production Go tracer used by deep province
// routes. Standard profiles never call it.
func NTraceProvinceTracer(ctx context.Context, target model.ProvinceLatencyTarget) ([]ProvinceRouteHop, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	network := "ip4"
	if strings.EqualFold(target.IPVersion, "ipv6") {
		network = "ip6"
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, network, target.Host)
	if err != nil {
		return nil, fmt.Errorf("resolve route target: %w", err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("route target has no %s address", target.IPVersion)
	}
	result, traceErr := trace.Traceroute(trace.TCPTrace, trace.Config{
		Context: ctx, BeginHop: 1, MaxHops: 30,
		NumMeasurements: 1, MaxAttempts: 1, ParallelRequests: 6,
		Timeout: time.Second, DstIP: addresses[0], DstPort: 80,
		PacketInterval: 50, TTLInterval: 50, PktSize: 52,
		IPGeoSource: ipgeo.GetSource("DISABLE-GEOIP"), Lang: "en",
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hops := provinceRouteHops(result)
	if traceErr != nil {
		return hops, traceErr
	}
	return hops, nil
}

func provinceRouteHops(result *trace.Result) []ProvinceRouteHop {
	if result == nil {
		return nil
	}
	hops := make([]ProvinceRouteHop, 0, len(result.Hops))
	for ttlIndex, attempts := range result.Hops {
		hop := ProvinceRouteHop{Hop: ttlIndex + 1}
		for _, attempt := range attempts {
			if attempt.TTL > 0 {
				hop.Hop = attempt.TTL
			}
			if !attempt.Success || attempt.Address == nil {
				continue
			}
			hop.Address = routeHopAddress(attempt.Address)
			hop.RTT = attempt.RTT
			if attempt.Geo != nil {
				hop.ASN = attempt.Geo.Asnumber
				hop.Location = strings.Join(strings.Fields(strings.Join([]string{attempt.Geo.Country, attempt.Geo.Prov, attempt.Geo.City}, " ")), " ")
			}
			break
		}
		hops = append(hops, hop)
	}
	return hops
}

func routeHopAddress(address net.Addr) string {
	switch value := address.(type) {
	case *net.IPAddr:
		return value.IP.String()
	case *net.TCPAddr:
		return value.IP.String()
	case *net.UDPAddr:
		return value.IP.String()
	default:
		return address.String()
	}
}
