package nt

import (
	"bytes"
	"context"
	"testing"

	"github.com/fatih/color"
	"github.com/nxtrace/NTrace-core/util"
)

func TestPrepareNextTraceAPIInfoUsesSilentCachedMetadata(t *testing.T) {
	oldIP := util.GetFastIPCache()
	oldMeta := util.GetFastIPMetaCache()
	t.Cleanup(func() { util.SetFastIPCacheState(oldIP, oldMeta) })

	util.SetFastIPCacheState("103.117.102.27", util.FastIPMeta{
		IP: "103.117.102.27", Latency: "1591.80", NodeName: "DMIT.NRT",
	})
	if got, want := prepareNextTraceAPIInfo(), "[NextTrace API] preferred API IP - 103.117.102.27 - 1591.80ms - DMIT.NRT"; got != want {
		t.Fatalf("API info = %q, want %q", got, want)
	}
}

func TestPreparedAPIInfoPreventsDirectFastIPOutput(t *testing.T) {
	oldIP := util.GetFastIPCache()
	oldMeta := util.GetFastIPMetaCache()
	oldOutput := color.Output
	t.Cleanup(func() {
		util.SetFastIPCacheState(oldIP, oldMeta)
		color.Output = oldOutput
	})

	util.SetFastIPCacheState("103.117.102.27", util.FastIPMeta{
		IP: "103.117.102.27", Latency: "12.34", NodeName: "fixture",
	})
	if prepareNextTraceAPIInfo() == "" {
		t.Fatal("prepared API info is empty")
	}
	var output bytes.Buffer
	color.Output = &output
	if _, err := util.GetFastIPWithContext(context.Background(), "api.nxtrace.org", "443", true); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("FastIP selection leaked direct output: %q", output.String())
	}
}

func TestPrepareNextTraceAPIInfoKeepsUsefulCachedIPWithoutMetadata(t *testing.T) {
	oldIP := util.GetFastIPCache()
	oldMeta := util.GetFastIPMetaCache()
	t.Cleanup(func() { util.SetFastIPCacheState(oldIP, oldMeta) })

	util.SetFastIPCacheState("203.0.113.7", util.FastIPMeta{})
	if got, want := prepareNextTraceAPIInfo(), "[NextTrace API] preferred API IP - 203.0.113.7"; got != want {
		t.Fatalf("API info = %q, want %q", got, want)
	}
}
