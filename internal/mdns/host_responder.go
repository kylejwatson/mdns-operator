package mdns

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/miekg/dns/rdata"
)

func (p *Publisher) serveMDNS(ctx context.Context, iface net.Interface) error {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
		return nil
	}

	conn, err := net.ListenMulticastUDP("udp4", &iface, &net.UDPAddr{IP: net.IPv4zero, Port: 5353})
	if err != nil {
		return fmt.Errorf("listen for mDNS on %s: %w", iface.Name, err)
	}
	defer conn.Close()

	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) {
		resp := p.buildResponse(req)
		if len(resp.Answer) == 0 {
			return
		}
		if _, err := io.Copy(w, resp); err != nil {
			p.log.Error(err, "failed to write mDNS response", "interface", iface.Name)
		}
	})}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("listen for mDNS on %s: %w", iface.Name, err)
	}
	return nil
}

func (p *Publisher) buildResponse(req *dns.Msg) *dns.Msg {
	resp := new(dns.Msg)
	if req == nil || len(req.Question) == 0 {
		return resp
	}
	if qclass := req.Question[0].Header().Class; qclass != dns.ClassINET {
		return resp
	}

	qname := req.Question[0].Header().Name
	qtype := dns.RRToType(req.Question[0])
	if qname == "" || qtype != dns.TypeA && qtype != dns.TypeAAAA {
		return resp
	}

	resp = dnsutil.SetReply(resp, req)
	resp.Authoritative = true
	resp.RecursionAvailable = true

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, state := range p.active {
		if strings.TrimSuffix(state.hostname, ".") != strings.TrimSuffix(qname, ".") {
			continue
		}
		parsed := net.ParseIP(state.ip)
		if parsed == nil {
			continue
		}
		if qtype == dns.TypeA && parsed.To4() != nil {
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.Header{Name: qname, Class: dns.ClassINET, TTL: 120},
				A:   rdata.A{Addr: netip.MustParseAddr(parsed.String())},
			})
		}
		if qtype == dns.TypeAAAA && parsed.To16() != nil && parsed.To4() == nil {
			resp.Answer = append(resp.Answer, &dns.AAAA{
				Hdr:  dns.Header{Name: qname, Class: dns.ClassINET, TTL: 120},
				AAAA: rdata.AAAA{Addr: netip.MustParseAddr(parsed.String())},
			})
		}
	}

	return resp
}
