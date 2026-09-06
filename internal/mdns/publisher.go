package mdns

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/grandcat/zeroconf"
)

type publishState struct {
	server   *zeroconf.Server
	hostname string
	ip       string
}

type Publisher struct {
	mu         sync.Mutex
	active     map[string]publishState
	log        logr.Logger
	interfaces []net.Interface
}

func NewPublisher(log logr.Logger) *Publisher {
	ifaces, ifaceLog := discoverPublishInterfaces(log)
	if len(ifaceLog) == 0 {
		log.Info("mDNS will use all interfaces selected by zeroconf")
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
		state.server.Shutdown()
		delete(p.active, key)
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid ip address %q", ip)
	}

	serverName, instance := serviceIdentityFromHostname(hostname)
	server, err := registerProxyWithRetry(p.log, hostname, instance, serverName, parsedIP, p.interfaces)
	if err != nil {
		return err
	}

	p.active[key] = publishState{server: server, hostname: hostname, ip: ip}
	p.log.Info("broadcasting mDNS record", "key", key, "hostname", hostname, "ip", ip)
	return nil
}

func (p *Publisher) Stop(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.active[key]
	if !ok {
		return
	}
	state.server.Shutdown()
	delete(p.active, key)
	p.log.Info("unregistered mDNS record", "key", key, "hostname", state.hostname)
}

func (p *Publisher) ShutdownAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, state := range p.active {
		state.server.Shutdown()
		delete(p.active, key)
	}
}
