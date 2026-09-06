package mdns

import (
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/grandcat/zeroconf"
)

const (
	mdnsServiceType   = "_http._tcp"
	mdnsDomain        = "local."
	mdnsPort          = 80
	mdnsMaxAttempts   = 5
	mdnsRetryInterval = time.Second
)

func registerProxyWithRetry(
	log logr.Logger,
	hostname string,
	instance string,
	serverName string,
	parsedIP net.IP,
	interfaces []net.Interface,
) (*zeroconf.Server, error) {
	var lastErr error
	for attempt := 1; attempt <= mdnsMaxAttempts; attempt++ {
		server, err := zeroconf.RegisterProxy(
			instance,
			mdnsServiceType,
			mdnsDomain,
			mdnsPort,
			serverName,
			[]string{parsedIP.String()},
			nil,
			interfaces,
		)
		if err == nil {
			if attempt > 1 {
				log.Info("publish succeeded after retry", "hostname", hostname, "attempt", attempt)
			}
			return server, nil
		}

		lastErr = err
		log.Error(err, "error publishing mDNS record", "hostname", hostname, "attempt", fmt.Sprintf("%d/%d", attempt, mdnsMaxAttempts))
		if attempt < mdnsMaxAttempts {
			time.Sleep(mdnsRetryInterval)
		}
	}

	return nil, fmt.Errorf("publish %s failed after retries: %w", hostname, lastErr)
}
