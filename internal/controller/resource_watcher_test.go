package controller

import (
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
)

type fakePublisher struct {
	starts        []publishCall
	stops         []string
	shutdownCount  int
}

type publishCall struct {
	key      string
	hostname string
	ip       string
}

func (f *fakePublisher) Start(key, hostname, ip string) error {
	f.starts = append(f.starts, publishCall{key: key, hostname: hostname, ip: ip})
	return nil
}

func (f *fakePublisher) Stop(key string) {
	f.stops = append(f.stops, key)
}

func (f *fakePublisher) ShutdownAll() {
	f.shutdownCount++
}

func TestResourceKey(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		namespace string
		resource  string
		want      string
	}{
		{name: "namespaced resource", prefix: "gateway", namespace: "default", resource: "demo", want: "gateway/default/demo"},
		{name: "cluster scoped resource", prefix: "node", namespace: "", resource: "worker-1", want: "node/worker-1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceKey(tc.prefix, tc.namespace, tc.resource); got != tc.want {
				t.Fatalf("resourceKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandleEventPublishesNodeRecord(t *testing.T) {
	publisher := &fakePublisher{}
	watcher := &resourceWatcher{publisher: publisher, log: logr.Discard(), keyPrefix: "node"}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]any{
			"name":        "worker-1",
			"annotations": map[string]any{hostnameAnnotationKey: "worker-1.local"},
		},
		"status": map[string]any{
			"addresses": []any{
				map[string]any{"type": "Hostname", "value": "worker-1"},
					map[string]any{"type": "InternalIP", "value": "10.0.0.11"},
			},
		},
	}}

	if err := watcher.handleEvent(watch.Event{Type: watch.Added, Object: obj}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}

	if got, want := len(publisher.starts), 1; got != want {
		t.Fatalf("publisher start count = %d, want %d", got, want)
	}
	call := publisher.starts[0]
	if got, want := call.key, "node/worker-1"; got != want {
		t.Fatalf("start key = %q, want %q", got, want)
	}
	if got, want := call.hostname, "worker-1.local"; got != want {
		t.Fatalf("start hostname = %q, want %q", got, want)
	}
	if got, want := call.ip, "10.0.0.11"; got != want {
		t.Fatalf("start ip = %q, want %q", got, want)
	}
}

func TestHandleEventStopsOnDelete(t *testing.T) {
	publisher := &fakePublisher{}
	watcher := &resourceWatcher{publisher: publisher, log: logr.Discard(), keyPrefix: "gateway"}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"namespace": "default",
			"name":      "demo-gateway",
		},
	}}

	if err := watcher.handleEvent(watch.Event{Type: watch.Deleted, Object: obj}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}

	if got, want := len(publisher.stops), 1; got != want {
		t.Fatalf("publisher stop count = %d, want %d", got, want)
	}
	if got, want := publisher.stops[0], "gateway/default/demo-gateway"; got != want {
		t.Fatalf("stop key = %q, want %q", got, want)
	}
}

func TestHandleEventStopsWhenIPMissing(t *testing.T) {
	publisher := &fakePublisher{}
	watcher := &resourceWatcher{publisher: publisher, log: logr.Discard(), keyPrefix: "node"}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":        "worker-2",
			"annotations": map[string]any{hostnameAnnotationKey: "worker-2.local"},
		},
		"status": map[string]any{
			"addresses": []any{map[string]any{"type": "Hostname", "value": "worker-2"}},
		},
	}}

	if err := watcher.handleEvent(watch.Event{Type: watch.Modified, Object: obj}); err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}

	if got, want := len(publisher.starts), 0; got != want {
		t.Fatalf("publisher start count = %d, want %d", got, want)
	}
	if got, want := len(publisher.stops), 1; got != want {
		t.Fatalf("publisher stop count = %d, want %d", got, want)
	}
	if got, want := publisher.stops[0], "node/worker-2"; got != want {
		t.Fatalf("stop key = %q, want %q", got, want)
	}
}
