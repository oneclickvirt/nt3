package nt

import (
	"net"
	"testing"
	"time"

	"github.com/nxtrace/NTrace-core/ipgeo"
	"github.com/nxtrace/NTrace-core/trace"
)

func TestProvinceRouteHopsNormalizesTraceResult(t *testing.T) {
	result := &trace.Result{Hops: [][]trace.Hop{
		{{TTL: 1, Success: false}},
		{{TTL: 2, Success: true, Address: &net.IPAddr{IP: net.ParseIP("192.0.2.1")}, RTT: 4 * time.Millisecond, Geo: &ipgeo.IPGeoData{Asnumber: "64500", Country: "US", City: "Fixture"}}},
	}}
	hops := provinceRouteHops(result)
	if len(hops) != 2 || hops[0].Hop != 1 || hops[0].Address != "" {
		t.Fatalf("unexpected timeout hop: %+v", hops)
	}
	if hops[1].Hop != 2 || hops[1].Address != "192.0.2.1" || hops[1].ASN != "64500" || hops[1].Location != "US Fixture" {
		t.Fatalf("unexpected successful hop: %+v", hops[1])
	}
}
