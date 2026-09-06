//go:build !linux

package mdns

func resolveDefaultRouteIndex() int {
	return 0
}
