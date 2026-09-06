package mdns

import (
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"
)

func discoverPublishInterfaces(log logr.Logger) ([]net.Interface, []string) {
	requested := strings.TrimSpace(os.Getenv("MDNS_INTERFACE"))
	if requested == "" {
		ifaces, names := defaultPublishInterfaces(log)
		return ifaces, names
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
		ifaces, selected := defaultPublishInterfaces(log)
		return ifaces, selected
	}

	all, err := net.Interfaces()
	if err != nil {
		log.Error(err, "failed to enumerate network interfaces, using zeroconf default")
		return nil, nil
	}

	selected := make([]net.Interface, 0, len(byName))
	selectedNames := make([]string, 0, len(byName))
	for _, iface := range all {
		if _, ok := byName[iface.Name]; !ok {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		selected = append(selected, iface)
		selectedNames = append(selectedNames, iface.Name)
	}

	if len(selected) == 0 {
		log.Info("no usable interfaces matched MDNS_INTERFACE, using default interface selection", "requested", requested)
		ifaces, names := defaultPublishInterfaces(log)
		return ifaces, names
	}

	return selected, selectedNames
}

func defaultPublishInterfaces(log logr.Logger) ([]net.Interface, []string) {
	all, err := net.Interfaces()
	if err != nil {
		log.Error(err, "failed to enumerate network interfaces, using zeroconf default")
		return nil, nil
	}

	selected := make([]net.Interface, 0, len(all))
	selectedNames := make([]string, 0, len(all))
	for _, iface := range all {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		selected = append(selected, iface)
		selectedNames = append(selectedNames, iface.Name)
	}

	if len(selected) == 0 {
		return nil, nil
	}

	return selected, selectedNames
}
