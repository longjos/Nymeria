package kisstcp

import (
	"errors"
	"testing"
)

func TestMergeDiscoverEmpty(t *testing.T) {
	got := mergeDiscover(nil, false)
	if got == nil {
		t.Fatal("nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestMergeDiscoverLocalOnly(t *testing.T) {
	got := mergeDiscover(nil, true)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Host != "localhost" || got[0].Port != DefaultKISSPort || !got[0].Local {
		t.Errorf("local = %+v", got[0])
	}
	if got[0].Source != sourceLocal {
		t.Errorf("source = %q", got[0].Source)
	}
	if !got[0].Highlight {
		t.Error("local probe should highlight")
	}
	if got[0].Label == "" || got[0].Name == "" {
		t.Error("missing label/name")
	}
}

func TestMergeDiscoverMDNS(t *testing.T) {
	got := mergeDiscover([]rawTNC{{
		Instance:  "Dire Wolf on radiopi",
		Host:      "192.168.1.40",
		Port:      8001,
		PortsNote: "144.800 1200 bit/s",
	}}, false)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	p := got[0]
	if p.Host != "192.168.1.40" || p.Port != 8001 {
		t.Errorf("addr = %s:%d", p.Host, p.Port)
	}
	if p.Source != sourceMDNS {
		t.Errorf("source = %q", p.Source)
	}
	if !p.Highlight {
		t.Error("mdns should highlight")
	}
	if p.PortsNote != "144.800 1200 bit/s" {
		t.Errorf("pn = %q", p.PortsNote)
	}
	if p.Label == "Dire Wolf on radiopi — 192.168.1.40:8001" {
		// exact
	} else if p.Name != "Dire Wolf on radiopi" {
		t.Errorf("name/label = %q / %q", p.Name, p.Label)
	}
}

func TestMergeDiscoverDedupeLocalAndLoopbackMDNS(t *testing.T) {
	got := mergeDiscover([]rawTNC{{
		Instance: "Dire Wolf on laptop",
		Host:     "127.0.0.1",
		Port:     8001,
	}}, true)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (deduped): %+v", len(got), got)
	}
	if !got[0].Local || got[0].Host != "localhost" {
		t.Errorf("prefer local row: %+v", got[0])
	}
}

func TestMergeDiscoverKeepsRemoteAndLocal(t *testing.T) {
	got := mergeDiscover([]rawTNC{{
		Instance: "Dire Wolf on radiopi",
		Host:     "192.168.1.40",
		Port:     8001,
	}}, true)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].Local {
		t.Errorf("local should sort first: %+v", got[0])
	}
}

func TestMergeDiscoverSkipsEmptyHost(t *testing.T) {
	got := mergeDiscover([]rawTNC{{Instance: "nameless", Port: 8001}}, false)
	if len(got) != 0 {
		t.Fatalf("empty host should be dropped: %+v", got)
	}
}

func TestMergeDiscoverDefaultPort(t *testing.T) {
	got := mergeDiscover([]rawTNC{{Instance: "x", Host: "10.0.0.2"}}, false)
	if len(got) != 1 || got[0].Port != DefaultKISSPort {
		t.Fatalf("default port: %+v", got)
	}
}

func TestNormalizeKISSHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1", "localhost"},
		{"::1", "localhost"},
		{"[::1]", "localhost"},
		{"LOCALHOST", "localhost"},
		{"192.168.1.5", "192.168.1.5"},
		{" Radiopi.local. ", "radiopi.local"},
	}
	for _, tc := range cases {
		if got := normalizeKISSHost(tc.in); got != tc.want {
			t.Errorf("normalizeKISSHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSameKISSAddr(t *testing.T) {
	if !sameKISSAddr("127.0.0.1", 8001, "localhost", 8001) {
		t.Error("loopback should match localhost")
	}
	if sameKISSAddr("192.168.1.5", 8001, "192.168.1.5", 8002) {
		t.Error("port mismatch")
	}
}

func TestDiscoverUsesHooks(t *testing.T) {
	prevB, prevP := browseMDNS, probeLocal
	t.Cleanup(func() {
		browseMDNS = prevB
		probeLocal = prevP
	})
	browseMDNS = func() ([]rawTNC, error) {
		return []rawTNC{{Instance: "Dire Wolf on pi", Host: "10.0.0.8", Port: 8001}}, nil
	}
	probeLocal = func() bool { return false }

	got, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Host != "10.0.0.8" {
		t.Fatalf("%+v", got)
	}
}

func TestDiscoverMDNSErrorStillReturnsLocal(t *testing.T) {
	prevB, prevP := browseMDNS, probeLocal
	t.Cleanup(func() {
		browseMDNS = prevB
		probeLocal = prevP
	})
	browseMDNS = func() ([]rawTNC, error) {
		return nil, errors.New("no multicast")
	}
	probeLocal = func() bool { return true }

	got, err := Discover()
	if err == nil || err.Error() != "no multicast" {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || !got[0].Local {
		t.Fatalf("want local fallback, got %+v", got)
	}
}

func TestSetDiscoverForTest(t *testing.T) {
	restore := SetDiscoverForTest(func() ([]TNCInfo, error) {
		return []TNCInfo{{Name: "stub", Host: "h", Port: 1}}, nil
	})
	t.Cleanup(restore)
	got, err := Discover()
	if err != nil || len(got) != 1 || got[0].Name != "stub" {
		t.Fatalf("%v %+v", err, got)
	}
}
