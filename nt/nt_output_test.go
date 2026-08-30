package nt

import (
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
