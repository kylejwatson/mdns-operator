package controller

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var nodeGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "nodes",
}

type NodeWatcher struct {
	*resourceWatcher
}

func NewNodeWatcher(cfg *rest.Config, publisher mDNSPublisher, log logr.Logger) (*NodeWatcher, error) {
	watcher, err := newResourceWatcher(cfg, publisher, log, nodeGVR, "Nodes", "node")
	if err != nil {
		return nil, fmt.Errorf("construct node watcher: %w", err)
	}

	return &NodeWatcher{resourceWatcher: watcher}, nil
}