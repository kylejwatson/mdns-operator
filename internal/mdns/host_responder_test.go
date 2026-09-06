package mdns

import (
	"net"
	"testing"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

func TestPickBestInterface(t *testing.T) {
	ifaces := []net.Interface{
		{Name: "docker0", Index: 1, Flags: net.FlagUp | net.FlagMulticast, HardwareAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}},
		{Name: "br0", Index: 2, Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast, HardwareAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x66}},
		{Name: "eno1", Index: 3, Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast, HardwareAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x77}},
	}

	got, ok := pickBestInterface(ifaces, 3)
	if !ok {
		t.Fatal("expected to choose a usable interface")
	}
	if got.Name != "eno1" {
		t.Fatalf("expected eno1, got %q", got.Name)
	}
}

func TestPickBestInterfacePrefersHardwareAndDefaultRoute(t *testing.T) {
	ifaces := []net.Interface{
		{Name: "docker0", Index: 1, Flags: net.FlagUp | net.FlagMulticast, HardwareAddr: nil},
		{Name: "br0", Index: 2, Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast, HardwareAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}},
		{Name: "eno1", Index: 3, Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast, HardwareAddr: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x66}},
	}

	got, ok := pickBestInterface(ifaces, 3)
	if !ok {
		t.Fatal("expected to choose a usable interface")
	}
	if got.Name != "eno1" {
		t.Fatalf("expected eno1, got %q", got.Name)
	}
}

func TestPublisherBuildResponse(t *testing.T) {
	publisher := &Publisher{
		active: map[string]publishState{
			"default/demo":  {hostname: "demo.local", ip: "10.0.0.42"},
			"default/demo6": {hostname: "demo6.local", ip: "2001:db8::42"},
		},
	}

	t.Run("a record", func(t *testing.T) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "demo.local.", dns.TypeA)
		resp := publisher.buildResponse(msg)
		if len(resp.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
		}
		answer, ok := resp.Answer[0].(*dns.A)
		if !ok {
			t.Fatalf("expected A record, got %T", resp.Answer[0])
		}
		if got, want := answer.A.String(), "10.0.0.42"; got != want {
			t.Fatalf("A record mismatch: got %q want %q", got, want)
		}
	})

	t.Run("aaaa record", func(t *testing.T) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "demo6.local.", dns.TypeAAAA)
		resp := publisher.buildResponse(msg)
		if len(resp.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
		}
		answer, ok := resp.Answer[0].(*dns.AAAA)
		if !ok {
			t.Fatalf("expected AAAA record, got %T", resp.Answer[0])
		}
		if got, want := answer.AAAA.String(), "2001:db8::42"; got != want {
			t.Fatalf("AAAA record mismatch: got %q want %q", got, want)
		}
	})

	t.Run("non address query ignored", func(t *testing.T) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "demo.local.", dns.TypeTXT)
		resp := publisher.buildResponse(msg)
		if len(resp.Answer) != 0 {
			t.Fatalf("expected no answers for TXT record, got %d", len(resp.Answer))
		}
	})

	t.Run("non local hostname ignored", func(t *testing.T) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
		resp := publisher.buildResponse(msg)
		if len(resp.Answer) != 0 {
			t.Fatalf("expected no answers for non-local hostname, got %d", len(resp.Answer))
		}
	})
}
