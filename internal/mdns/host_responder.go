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
	"golang.org/x/net/ipv4"
)

const mdnsQuestionClassMask = 0x7fff
const mdnsQuestionUnicastResponseBit = 0x8000

type responseDelivery struct {
	unicast   bool
	multicast bool
}

func (p *Publisher) serveMDNS(ctx context.Context) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 5353})
	if err != nil {
		return fmt.Errorf("listen for mDNS: %w", err)
	}
	defer conn.Close()

	packetConn := ipv4.NewPacketConn(conn)
	mcastGroup := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	if mcastGroup.IP == nil {
		return fmt.Errorf("invalid mDNS multicast group")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("enumerate interfaces for mDNS: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if err := packetConn.JoinGroup(&iface, mcastGroup); err != nil {
			p.log.V(1).Info("failed to join mDNS multicast group on interface", "interface", iface.Name, "err", err)
			continue
		}
	}

	server := &dns.Server{PacketConn: conn, MsgAcceptFunc: acceptMDNSMessage, Handler: dns.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) {
		targets := responseTargets(req)
		if p.debugEnabled() {
			p.logRequest(w, req)
		}
		resp := p.buildResponse(req)
		if p.debugEnabled() {
			p.logResponse(w, req, resp, targets)
		}
		if len(resp.Answer) == 0 {
			return
		}
		if err := resp.Pack(); err != nil {
			p.log.Error(err, "failed to pack mDNS response")
			return
		}
		raw := resp.Data

		if targets.multicast {
			if _, err := conn.WriteToUDP(raw, mcastGroup); err != nil {
				p.log.Error(err, "failed to write multicast mDNS response")
			}
		}
		if targets.unicast {
			if _, err := io.Copy(w, resp); err != nil {
				p.log.Error(err, "failed to write unicast mDNS response")
			}
		}
	})}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("listen for mDNS: %w", err)
	}
	return nil
}

func (p *Publisher) logRequest(w dns.ResponseWriter, req *dns.Msg) {
	if req == nil {
		p.log.Info("received mDNS request", "remote", w.RemoteAddr().String(), "local", w.LocalAddr().String(), "questions", 0)
		return
	}

	questions := make([]string, 0, len(req.Question))
	for _, question := range req.Question {
		questions = append(questions, summarizeQuestion(question))
	}

	p.log.Info(
		"received mDNS request",
		"id", req.ID,
		"remote", w.RemoteAddr().String(),
		"local", w.LocalAddr().String(),
		"questions", len(req.Question),
		"questionDetails", questions,
	)
}

func (p *Publisher) logResponse(w dns.ResponseWriter, req, resp *dns.Msg, targets responseDelivery) {
	answers := make([]string, 0)
	if resp != nil {
		answers = make([]string, 0, len(resp.Answer))
		for _, answer := range resp.Answer {
			answers = append(answers, summarizeAnswer(answer))
		}
	}

	requestID := uint16(0)
	if req != nil {
		requestID = req.ID
	}

	p.log.Info(
		"sending mDNS response",
		"id", requestID,
		"remote", w.RemoteAddr().String(),
		"local", w.LocalAddr().String(),
		"answers", len(answers),
		"deliveryUnicast", targets.unicast,
		"deliveryMulticast", targets.multicast,
		"answerDetails", answers,
	)
}

func responseTargets(req *dns.Msg) responseDelivery {
	targets := responseDelivery{}
	if req == nil {
		return targets
	}

	for _, question := range req.Question {
		class := question.Header().Class
		if class&mdnsQuestionClassMask != dns.ClassINET {
			continue
		}
		if class&mdnsQuestionUnicastResponseBit != 0 {
			targets.unicast = true
			continue
		}
		targets.multicast = true
	}

	return targets
}

func summarizeQuestion(question dns.RR) string {
	if question == nil {
		return "<nil>"
	}

	hdr := question.Header()
	questionType := dns.RRToType(question)
	return fmt.Sprintf(
		"name=%s type=%s class=%d baseClass=%d",
		hdr.Name,
		typeName(questionType),
		hdr.Class,
		hdr.Class&mdnsQuestionClassMask,
	)
}

func summarizeAnswer(answer dns.RR) string {
	if answer == nil {
		return "<nil>"
	}

	hdr := answer.Header()
	return fmt.Sprintf(
		"name=%s type=%s class=%d ttl=%d data=%s",
		hdr.Name,
		typeName(dns.RRToType(answer)),
		hdr.Class,
		hdr.TTL,
		answer.String(),
	)
}

func typeName(rrType uint16) string {
	if name, ok := dns.TypeToString[rrType]; ok {
		return name
	}
	return fmt.Sprintf("TYPE%d", rrType)
}

func acceptMDNSMessage(msg *dns.Msg) dns.MsgAcceptAction {
	if msg.Response {
		return dns.MsgIgnore
	}
	if _, ok := dns.OpcodeToString[msg.Opcode]; !ok {
		return dns.MsgRejectNotImplemented
	}
	if len(msg.Question) == 0 {
		return dns.MsgReject
	}
	for _, question := range msg.Question {
		if _, ok := question.(*dns.RRSIG); ok {
			return dns.MsgRejectRefused
		}
	}
	return dns.MsgAccept
}

func (p *Publisher) buildResponse(req *dns.Msg) *dns.Msg {
	resp := new(dns.Msg)
	if req == nil || len(req.Question) == 0 {
		return resp
	}

	resp = dnsutil.SetReply(resp, req)
	resp.Authoritative = true
	resp.RecursionAvailable = false

	p.mu.Lock()
	defer p.mu.Unlock()

	seen := make(map[string]struct{})
	for _, question := range req.Question {
		qclass := question.Header().Class & mdnsQuestionClassMask
		if qclass != dns.ClassINET {
			continue
		}

		qname := question.Header().Name
		qtype := dns.RRToType(question)
		if qname == "" || qtype != dns.TypeA && qtype != dns.TypeAAAA {
			continue
		}

		for _, state := range p.active {
			if strings.TrimSuffix(state.hostname, ".") != strings.TrimSuffix(qname, ".") {
				continue
			}
			parsed := net.ParseIP(state.ip)
			if parsed == nil {
				continue
			}

			key := fmt.Sprintf("%s|%d|%s", strings.TrimSuffix(qname, "."), qtype, parsed.String())
			if _, ok := seen[key]; ok {
				continue
			}

			if qtype == dns.TypeA && parsed.To4() != nil {
				resp.Answer = append(resp.Answer, &dns.A{
					Hdr: dns.Header{Name: qname, Class: dns.ClassINET, TTL: 120},
					A:   rdata.A{Addr: netip.MustParseAddr(parsed.String())},
				})
				seen[key] = struct{}{}
			}
			if qtype == dns.TypeAAAA && parsed.To16() != nil && parsed.To4() == nil {
				resp.Answer = append(resp.Answer, &dns.AAAA{
					Hdr:  dns.Header{Name: qname, Class: dns.ClassINET, TTL: 120},
					AAAA: rdata.AAAA{Addr: netip.MustParseAddr(parsed.String())},
				})
				seen[key] = struct{}{}
			}
		}
	}

	return resp
}
