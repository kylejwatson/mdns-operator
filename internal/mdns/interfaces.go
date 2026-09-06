package mdns

import (
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"
)

func discoverPublishInterface(log logr.Logger) (net.Interface, bool) {
	requested := strings.TrimSpace(os.Getenv("MDNS_INTERFACE"))
	if requested == "" {
		return defaultPublishInterface(log)
	}

	names := strings.Split(requested, ",")
	byName := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			byName[trimmed] = struct{}{}
		}
	}
	if len(byName) == 0 {
		log.Info("MDNS_INTERFACE was set but no valid names were found, using default interface selection")
		return defaultPublishInterface(log)
	}

	all, err := net.Interfaces()
	if err != nil {
		log.Error(err, "failed to enumerate network interfaces, using mDNS library default")
		return net.Interface{}, false
	}

	selected := make([]net.Interface, 0, len(byName))
	for _, iface := range all {
		if _, ok := byName[iface.Name]; !ok {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		selected = append(selected, iface)
	}

	if len(selected) == 0 {
		log.Info("no usable interfaces matched MDNS_INTERFACE, using default interface selection", "requested", requested)
		return defaultPublishInterface(log)
	}

	preferred := resolveDefaultRouteIndex()
	best, ok := pickBestInterface(selected, preferred)
	if ok {
		return best, true
	}
	return selected[0], true
}

func defaultPublishInterface(log logr.Logger) (net.Interface, bool) {
	all, err := net.Interfaces()
	if err != nil {
		log.Error(err, "failed to enumerate network interfaces, using mDNS library default")
		return net.Interface{}, false
	}

	preferred := resolveDefaultRouteIndex()
	best, ok := pickBestInterface(all, preferred)
	if !ok {
		return net.Interface{}, false
	}

	log.Info("selected LAN-facing interface for mDNS", "interface", best.Name)
	return best, true
}

func pickBestInterface(ifaces []net.Interface, defaultRouteIndex int) (net.Interface, bool) {
	if defaultRouteIndex > 0 {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
				continue
			}
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if len(iface.HardwareAddr) == 0 {
				continue
			}
			if iface.Index == defaultRouteIndex {
				return iface, true
			}
		}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		if iface.Flags&net.FlagBroadcast != 0 {
			return iface, true
		}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		return iface, true
	}

	return net.Interface{}, false
}
