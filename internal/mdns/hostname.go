package mdns

import "strings"

func normalizeHostname(hostname string) string {
	if !strings.HasSuffix(hostname, ".local") {
		hostname += ".local"
	}
	return hostname
}

func serviceIdentityFromHostname(hostname string) (serverName string, instance string) {
	serverName = hostname
	if !strings.HasSuffix(serverName, ".") {
		serverName += "."
	}

	instance = strings.TrimSuffix(hostname, ".local")
	instance = strings.TrimSuffix(instance, ".")
	return serverName, instance
}
