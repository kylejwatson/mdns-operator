//go:build linux

package mdns

import (
	"github.com/vishvananda/netlink"
)

func resolveDefaultRouteIndex() int {
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		routes, err := netlink.RouteList(nil, family)
		if err != nil {
			continue
		}
		for _, route := range routes {
			if route.LinkIndex == 0 {
				continue
			}
			if route.Dst == nil || route.Dst.String() == "0.0.0.0/0" || route.Dst.String() == "::/0" {
				return route.LinkIndex
			}
		}
	}
	return 0
}
