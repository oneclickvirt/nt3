package nt

import (
	"strings"
	"testing"

	"github.com/nxtrace/NTrace-core/trace"
)

func TestAppendTraceStopReasonUsesNTraceCoreReason(t *testing.T) {
	buffer := &OutputBuffer{}
	appendTraceStopReason(buffer, &trace.Result{StopReason: &trace.StopReason{
		Hop:       4,
		Reason:    trace.StopReasonUnreachable,
		Responses: []string{"ICMP Host Unreachable"},
		Markers:   []string{"!H"},
	}})

	got := buffer.GetAll()
	if len(got) != 1 || got[0] != "Trace Stopped: No Continuing Route Observed at Hop 4 (ICMP Host Unreachable (!H))" {
		t.Fatalf("unexpected stop reason output: %#v", got)
	}
}

func TestIsTraceHeaderDoesNotMatchICMPStopReason(t *testing.T) {
	if !IsTraceHeader("\x1b[33m\x1b[01m广州移动 - ICMP v4 -\x1b[0m") {
		t.Fatal("colored carrier header was not recognized")
	}
	if !IsTraceHeader("广州移动 - icmp V6 -") {
		t.Fatal("case-insensitive carrier header was not recognized")
	}
	if IsTraceHeader("Trace Stopped: Destination Reached at Hop 19 (ICMP Echo Reply)") {
		t.Fatal("trace stop reason was mistaken for a carrier header")
	}
}

func TestFormatTraceOutputSeparatesStopReasonAndNextHeader(t *testing.T) {
	lines := []string{
		"\x1b[33m\x1b[01m广州电信 - ICMP v4 -\x1b[0m",
		"traceroute to 58.60.188.222, 30 hops max, 52 byte packets",
		"1.00 ms AS4134 hop",
		"Trace Stopped: Destination Reached at Hop 3 (ICMP Echo Reply)",
		"\x1b[33m\x1b[01m广州移动 - ICMP v4 -\x1b[0m",
		"traceroute to 120.196.165.24, 30 hops max, 52 byte packets",
	}

	got := FormatTraceOutput(lines)
	if !strings.Contains(got, "ICMP Echo Reply)\n") {
		t.Fatalf("stop reason was not terminated: %q", got)
	}
	if strings.Contains(got, "ICMP Echo Reply)广州移动") {
		t.Fatalf("next carrier header was concatenated: %q", got)
	}
	wantSuffix := "广州移动 - ICMP v4 -\x1b[0mtraceroute to 120.196.165.24, 30 hops max, 52 byte packets\n"
	if !strings.Contains(got, wantSuffix) {
		t.Fatalf("header and traceroute body were not joined: %q", got)
	}
}

func TestFormatTraceOutputTerminatesHeaderOnlyAndErrorLines(t *testing.T) {
	got := FormatTraceOutput([]string{
		"广州电信 - ICMP v4 -",
		"Error: ICMP traceroute unavailable",
	})
	if got != "广州电信 - ICMP v4 -\nError: ICMP traceroute unavailable\n" {
		t.Fatalf("unexpected header/error formatting: %q", got)
	}
}
