package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/brutella/dnssd"
	dnssdlog "github.com/brutella/dnssd/log"
	"github.com/go-logr/logr"
)

type publishState struct {
	hostname string
	ip       string
	handle   dnssd.ServiceHandle
}

type Publisher struct {
	mu        sync.Mutex
	active    map[string]publishState
	log       logr.Logger
	cancel    context.CancelFunc
	responder dnssd.Responder
	debug     bool
}

func NewPublisher(log logr.Logger) *Publisher {
	debug := false
	if value, ok := os.LookupEnv("MDNS_RESPONDER_DEBUG"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			log.Error(err, "invalid MDNS_RESPONDER_DEBUG value", "value", value)
		} else {
			debug = parsed
		}
	}
	if debug {
		dnssdlog.Debug.Enable()
	}
	log.Info("mDNS will announce records using dnssd responder")
	return &Publisher{
		active: make(map[string]publishState),
		log:    log,
		debug:  debug,
	}
}

func (p *Publisher) Start(key, hostname, ip string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	hostname = normalizeHostname(hostname)

	if state, ok := p.active[key]; ok {
		if state.hostname == hostname && state.ip == ip {
			return nil
		}
		p.removeRegisteredLocked(state)
		delete(p.active, key)
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid ip address %q", ip)
	}

	if err := p.ensureResponderLocked(); err != nil {
		return err
	}

	service, err := buildService(hostname, parsedIP)
	if err != nil {
		return err
	}

	handle, err := p.responder.Add(service)
	if err != nil {
		return fmt.Errorf("register mDNS service for %q: %w", hostname, err)
	}

	p.active[key] = publishState{hostname: hostname, ip: ip, handle: handle}
	p.log.Info("broadcasting mDNS record", "key", key, "hostname", hostname, "ip", ip)
	return nil
}


func buildService(hostname string, ip net.IP) (dnssd.Service, error) {
	host, instance := serviceIdentityFromHostname(hostname)
	return dnssd.NewService(dnssd.Config{
		Name:   instance,
		Type:   "_http._tcp",
		Domain: "local",
		Host:   host,
		IPs:    []net.IP{ip},
		Port:   80,
	})
}

func (p *Publisher) ensureResponderLocked() error {
	if p.responder != nil {
		return nil
	}

	responder, err := dnssd.NewResponder()
	if err != nil {
		return fmt.Errorf("create dnssd responder: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.responder = responder
	p.cancel = cancel

	go func() {
		if err := responder.Respond(ctx); err != nil && !errors.Is(err, context.Canceled) {
			p.log.Error(err, "mDNS responder stopped")
		}
	}()

	return nil
}


func (p *Publisher) removeRegisteredLocked(state publishState) {
	if p.responder != nil && state.handle != nil {
		p.responder.Remove(state.handle)
	}
}

func (p *Publisher) stopResponderLocked() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.responder = nil
}

func (p *Publisher) Stop(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.active[key]
	if !ok {
		return
	}
	p.removeRegisteredLocked(state)
	delete(p.active, key)
	p.log.Info("unregistered mDNS record", "key", key, "hostname", state.hostname)
	if len(p.active) == 0 {
		p.stopResponderLocked()
	}
}

func (p *Publisher) ShutdownAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, state := range p.active {
		p.removeRegisteredLocked(state)
		delete(p.active, key)
	}
	p.stopResponderLocked()
}
