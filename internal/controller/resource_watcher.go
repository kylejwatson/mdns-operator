package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const hostnameAnnotationKey = "mdns.alpha.kubernetes.io/hostname"

type mDNSPublisher interface {
	Start(key, hostname, ip string) error
	Stop(key string)
	ShutdownAll()
}

type resourceWatcher struct {
	client     dynamic.Interface
	publisher  mDNSPublisher
	log        logr.Logger
	gvr        schema.GroupVersionResource
	watchLabel string
	keyPrefix  string
}

func newResourceWatcher(cfg *rest.Config, publisher mDNSPublisher, log logr.Logger, gvr schema.GroupVersionResource, watchLabel, keyPrefix string) (*resourceWatcher, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &resourceWatcher{
		client:     dyn,
		publisher:  publisher,
		log:        log,
		gvr:        gvr,
		watchLabel: watchLabel,
		keyPrefix:  keyPrefix,
	}, nil
}

func (r *resourceWatcher) Start(ctx context.Context) error {
	r.log.Info("watching "+r.watchLabel+" with mDNS hostname annotation")
	defer r.publisher.ShutdownAll()

	attempt := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		err := r.watchOnce(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		attempt++
		seconds := min(1<<min(attempt, 5), 30)
		backoff := time.Duration(seconds) * time.Second
		r.log.Error(err, "watch ended, reconnecting", "backoff", backoff.String())

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
	}
}

func (r *resourceWatcher) NeedLeaderElection() bool {
	return true
}

func (r *resourceWatcher) watchOnce(ctx context.Context) error {
	stream, err := r.client.Resource(r.gvr).Namespace(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{})
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
			if err := r.handleEvent(event); err != nil {
				r.log.Error(err, "event handling failed")
			}
		}
	}
}

func (r *resourceWatcher) handleEvent(event watch.Event) error {
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("unexpected object type %T", event.Object)
	}

	key := resourceKey(r.keyPrefix, obj.GetNamespace(), obj.GetName())
	if event.Type == watch.Deleted {
		r.publisher.Stop(key)
		return nil
	}

	hostname := obj.GetAnnotations()[hostnameAnnotationKey]
	if hostname == "" {
		r.publisher.Stop(key)
		return nil
	}

	ip, ok := extractStatusIPAddress(obj.Object)
	if !ok {
		r.publisher.Stop(key)
		return nil
	}

	return r.publisher.Start(key, hostname, ip)
}

func resourceKey(prefix, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("%s/%s", prefix, name)
	}
	return fmt.Sprintf("%s/%s/%s", prefix, namespace, name)
}

func extractStatusIPAddress(object map[string]any) (string, bool) {
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
		if (addrType == "InternalIP" || addrType == "IPAddress") && value != "" {
			return value, true
		}
	}
	return "", false
}