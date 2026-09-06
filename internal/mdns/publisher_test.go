package mdns

import (
	"net"
	"testing"
)

func TestBuildService(t *testing.T) {
	ip := net.ParseIP("10.0.0.42")
	if ip == nil {
		t.Fatal("failed to parse test ip")
	}

	svc, err := buildService("k8s.local", ip)
	if err != nil {
		t.Fatalf("buildService returned error: %v", err)
	}

	if got, want := svc.Hostname(), "k8s.local."; got != want {
		t.Fatalf("hostname mismatch: got %q want %q", got, want)
	}
	if got, want := svc.ServiceName(), "_http._tcp.local."; got != want {
		t.Fatalf("service name mismatch: got %q want %q", got, want)
	}
	if got := len(svc.IPs); got != 1 {
		t.Fatalf("ip count mismatch: got %d want 1", got)
	}
	if got, want := svc.IPs[0].String(), "10.0.0.42"; got != want {
		t.Fatalf("ip mismatch: got %q want %q", got, want)
	}
}
