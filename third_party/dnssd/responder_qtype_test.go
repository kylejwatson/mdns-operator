package dnssd

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"
)

type captureConn struct{ sent []*Response }

func (c *captureConn) SendQuery(*Query) error { return nil }
func (c *captureConn) SendResponse(r *Response) error {
	c.sent = append(c.sent, r)
	return nil
}
func (c *captureConn) Read(context.Context) <-chan *Request { return nil }
func (c *captureConn) Drain(context.Context)                {}
func (c *captureConn) Close()                               {}

// hostnameReply runs a query for qname/qtype through the responder and returns
// the message as a client would decode it off the wire.
func hostnameReply(t *testing.T, qname string, qtype uint16, ips ...string) *dns.Msg {
	t.Helper()
	cfg := Config{Name: "k8s", Type: "_http._tcp", Domain: "local", Host: "k8s", Port: 80}
	return replyFor(t, cfg, qname, qtype, ips...)
}

func replyFor(t *testing.T, cfg Config, qname string, qtype uint16, ips ...string) *dns.Msg {
	t.Helper()
	for _, ip := range ips {
		cfg.IPs = append(cfg.IPs, net.ParseIP(ip))
	}
	srv, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}

	query := new(dns.Msg)
	query.SetQuestion(qname, qtype)
	req := &Request{
		msg:   query,
		from:  &net.UDPAddr{IP: net.ParseIP("192.168.68.62"), Port: 5353},
		iface: &net.Interface{Name: "en0"},
	}

	conn := &captureConn{}
	newResponder(conn).handleQuery(req, []*Service{&srv})
	if len(conn.sent) != 1 {
		t.Fatalf("got %d responses, want 1", len(conn.sent))
	}

	wire, err := conn.sent[0].msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	msg := new(dns.Msg)
	if err := msg.Unpack(wire); err != nil {
		t.Fatal(err)
	}
	return msg
}

func onlyType[T dns.RR](t *testing.T, section string, rrs []dns.RR) T {
	t.Helper()
	if len(rrs) != 1 {
		t.Fatalf("%s: got %v, want exactly one record", section, rrs)
	}
	rr, ok := rrs[0].(T)
	if !ok {
		t.Fatalf("%s: got %T (%v)", section, rrs[0], rrs[0])
	}
	return rr
}

func TestHostnameAQueryAnswersWithA(t *testing.T) {
	msg := hostnameReply(t, "k8s.local.", dns.TypeA, "192.168.69.51")

	a := onlyType[*dns.A](t, "answer", msg.Answer)
	if a.A.String() != "192.168.69.51" {
		t.Fatalf("got %v", a.A)
	}
	onlyType[*dns.NSEC](t, "additional", msg.Extra)
}

func TestHostnameAAAAQueryOnIPv4HostAssertsNonexistence(t *testing.T) {
	msg := hostnameReply(t, "k8s.local.", dns.TypeAAAA, "192.168.69.51")

	nsec := onlyType[*dns.NSEC](t, "answer", msg.Answer)
	if nsec.Hdr.Name != "k8s.local." || len(nsec.TypeBitMap) != 1 || nsec.TypeBitMap[0] != dns.TypeA {
		t.Fatalf("unexpected NSEC: %v", nsec)
	}
	if nsec.Hdr.Class&(1<<15) == 0 {
		t.Fatal("NSEC is missing the cache-flush bit")
	}

	a := onlyType[*dns.A](t, "additional", msg.Extra)
	if a.A.String() != "192.168.69.51" {
		t.Fatalf("got %v", a.A)
	}
}

func TestHostnameAAAAQueryAnswersWithAAAA(t *testing.T) {
	msg := hostnameReply(t, "k8s.local.", dns.TypeAAAA, "fd00::51")

	onlyType[*dns.AAAA](t, "answer", msg.Answer)
}

func TestHostnameAQueryOnIPv6HostAssertsNonexistence(t *testing.T) {
	msg := hostnameReply(t, "k8s.local.", dns.TypeA, "fd00::51")

	onlyType[*dns.NSEC](t, "answer", msg.Answer)
	onlyType[*dns.AAAA](t, "additional", msg.Extra)
}

func TestHostnameANYQueryAnswersWithAllAddresses(t *testing.T) {
	msg := hostnameReply(t, "k8s.local.", dns.TypeANY, "192.168.69.51", "fd00::51")

	if len(msg.Answer) != 2 {
		t.Fatalf("got %v, want A and AAAA", msg.Answer)
	}
}

func TestMappedAAAAAnswersWithMappedAddress(t *testing.T) {
	cfg := Config{Name: "k8s", Type: "_http._tcp", Domain: "local", Host: "k8s", Port: 80, MappedAAAA: true}
	msg := replyFor(t, cfg, "k8s.local.", dns.TypeAAAA, "192.168.69.51")

	aaaa := onlyType[*dns.AAAA](t, "answer", msg.Answer)
	if got, want := aaaa.AAAA.To16(), net.ParseIP("::ffff:192.168.69.51"); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(msg.Extra) != 2 {
		t.Fatalf("additional: got %v, want A and NSEC", msg.Extra)
	}
	if _, ok := msg.Extra[0].(*dns.A); !ok {
		t.Fatalf("additional: got %T first", msg.Extra[0])
	}
	nsec, ok := msg.Extra[1].(*dns.NSEC)
	if !ok || len(nsec.TypeBitMap) != 2 {
		t.Fatalf("additional: got %v, want NSEC for A and AAAA", msg.Extra[1])
	}
}

func TestMappedAAAAKeepsAQueryAnswer(t *testing.T) {
	cfg := Config{Name: "k8s", Type: "_http._tcp", Domain: "local", Host: "k8s", Port: 80, MappedAAAA: true}
	msg := replyFor(t, cfg, "k8s.local.", dns.TypeA, "192.168.69.51")

	a := onlyType[*dns.A](t, "answer", msg.Answer)
	if a.A.String() != "192.168.69.51" {
		t.Fatalf("got %v", a.A)
	}
}
