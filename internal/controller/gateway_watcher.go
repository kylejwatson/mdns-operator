package controller

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/kyle/mdns-operator/internal/mdns"
)

var gatewayGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "gateways",
}

type GatewayWatcher struct {
	*resourceWatcher
}

func NewGatewayWatcher(cfg *rest.Config, publisher *mdns.Publisher, log logr.Logger) (*GatewayWatcher, error) {
	watcher, err := newResourceWatcher(cfg, publisher, log, gatewayGVR, "Gateways", "gateway")
	if err != nil {
		return nil, fmt.Errorf("construct gateway watcher: %w", err)
	}

	return &GatewayWatcher{resourceWatcher: watcher}, nil
}

