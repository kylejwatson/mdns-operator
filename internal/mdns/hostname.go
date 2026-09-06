package mdns

import "strings"

func normalizeHostname(hostname string) string {
	if !strings.HasSuffix(hostname, ".local") {
		hostname += ".local"
	}
	return hostname
}

func serviceIdentityFromHostname(hostname string) (host string, instance string) {
	host = strings.TrimSuffix(hostname, ".local")
	host = strings.TrimSuffix(host, ".")
	instance = host
	return host, instance
}
