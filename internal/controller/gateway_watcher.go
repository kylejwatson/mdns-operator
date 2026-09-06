package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/kyle/mdns-operator/internal/mdns"
)

var gatewayGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "gateways",
}

type GatewayWatcher struct {
	client    dynamic.Interface
	publisher *mdns.Publisher
	log       logr.Logger
}

func NewGatewayWatcher(cfg *rest.Config, log logr.Logger) (*GatewayWatcher, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &GatewayWatcher{
		client:    dyn,
		publisher: mdns.NewPublisher(log.WithName("mdns")),
		log:       log,
	}, nil
}

func (g *GatewayWatcher) Start(ctx context.Context) error {
	g.log.Info("watching Gateways with mDNS hostname annotation")
	defer g.publisher.ShutdownAll()

	attempt := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		err := g.watchOnce(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		attempt++
		seconds := 1 << min(attempt, 5)
		if seconds > 30 {
			seconds = 30
		}
		backoff := time.Duration(seconds) * time.Second
		g.log.Error(err, "watch ended, reconnecting", "backoff", backoff.String())

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
	}
}

func (g *GatewayWatcher) NeedLeaderElection() bool {
	return true
}

func (g *GatewayWatcher) watchOnce(ctx context.Context) error {
	stream, err := g.client.Resource(gatewayGVR).Namespace(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("start watch: %w", err)
	}
	defer stream.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-stream.ResultChan():
			if !ok {
				return errors.New("watch channel closed")
			}
			if err := g.handleEvent(event); err != nil {
				g.log.Error(err, "event handling failed")
			}
		}
	}
}

func (g *GatewayWatcher) handleEvent(event watch.Event) error {
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("unexpected object type %T", event.Object)
	}

	key := fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
	if event.Type == watch.Deleted {
		g.publisher.Stop(key)
		return nil
	}

	hostname := obj.GetAnnotations()["mdns.alpha.kubernetes.io/hostname"]
	if hostname == "" {
		g.publisher.Stop(key)
		return nil
	}

	ip, ok := extractGatewayIP(obj.Object)
	if !ok {
		g.publisher.Stop(key)
		return nil
	}

	return g.publisher.Start(key, hostname, ip)
}

func extractGatewayIP(object map[string]any) (string, bool) {
	addresses, found, err := unstructured.NestedSlice(object, "status", "addresses")
	if err != nil || !found {
		return "", false
	}

	for _, raw := range addresses {
		addr, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		addrType, _ := addr["type"].(string)
		value, _ := addr["value"].(string)
		if addrType == "IPAddress" && value != "" {
			return value, true
		}
	}
	return "", false
}

var _ runtime.Object = (*unstructured.Unstructured)(nil)
