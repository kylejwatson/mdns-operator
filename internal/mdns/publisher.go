package mdns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/go-logr/logr"
)

type publishState struct {
	hostname string
	ip       string
}

type Publisher struct {
	mu         sync.Mutex
	active     map[string]publishState
	log        logr.Logger
	interfaces []net.Interface
	cancel     context.CancelFunc
}

func NewPublisher(log logr.Logger) *Publisher {
	ifaces, ifaceLog := discoverPublishInterfaces(log)
	if len(ifaceLog) == 0 {
		log.Info("mDNS will use all usable interfaces")
	} else {
		log.Info("mDNS publish interfaces selected", "interfaces", strings.Join(ifaceLog, ","))
	}

	return &Publisher{
		active:     make(map[string]publishState),
		log:        log,
		interfaces: ifaces,
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
		delete(p.active, key)
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid ip address %q", ip)
	}
	_ = parsedIP

	if err := p.ensureResponder(); err != nil {
		return err
	}

	p.active[key] = publishState{hostname: hostname, ip: ip}
	p.log.Info("broadcasting mDNS record", "key", key, "hostname", hostname, "ip", ip)
	return nil
}

func (p *Publisher) ensureResponder() error {
	if p.cancel != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	for _, iface := range p.interfaces {
		go func() {
			if err := p.serveMDNS(ctx, iface); err != nil && ctx.Err() == nil {
				p.log.Error(err, "mDNS responder stopped", "interface", iface.Name)
			}
		}()
	}
	if len(p.interfaces) == 0 {
		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				go func() {
					if err := p.serveMDNS(ctx, iface); err != nil && ctx.Err() == nil {
						p.log.Error(err, "mDNS responder stopped", "interface", iface.Name)
					}
				}()
			}
		}
	}

	return nil
}

func (p *Publisher) Stop(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.active[key]
	if !ok {
		return
	}
	delete(p.active, key)
	p.log.Info("unregistered mDNS record", "key", key, "hostname", state.hostname)
	if len(p.active) == 0 && p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

func (p *Publisher) ShutdownAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key := range p.active {
		delete(p.active, key)
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}
