package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/bobbydrake/flux-reconcile/pkg/output"
)

type Monitor struct {
	kind          string
	name          string
	namespace     string
	clientset     *kubernetes.Clientset
	dynamicClient dynamic.Interface
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	lastHash      string
	ready         bool
	readyMu       sync.RWMutex
}

func NewMonitor(ctx context.Context, kind, name, namespace string) (*Monitor, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	monitorCtx, cancel := context.WithCancel(ctx)

	return &Monitor{
		kind:          kind,
		name:          name,
		namespace:     namespace,
		clientset:     clientset,
		dynamicClient: dynamicClient,
		ctx:           monitorCtx,
		cancel:        cancel,
	}, nil
}

func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig file
	config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (m *Monitor) Watch() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkEvents()
		}
	}
}

func (m *Monitor) checkEvents() {
	fieldSelector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", m.name),
		fields.OneTermEqualSelector("involvedObject.namespace", m.namespace),
	).String()

	events, err := m.clientset.CoreV1().Events(m.namespace).List(m.ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
		Limit:         10,
	})

	if err != nil {
		return
	}

	// Get the most recent events
	if len(events.Items) == 0 {
		return
	}

	// Create a hash of recent events to detect changes
	hash := ""
	for i := len(events.Items) - 1; i >= 0 && i >= len(events.Items)-3; i-- {
		evt := events.Items[i]
		hash += fmt.Sprintf("%s:%s:%s", evt.Reason, evt.Type, evt.Message)
	}

	m.mu.Lock()
	if hash != m.lastHash {
		m.lastHash = hash
		m.mu.Unlock()

		// Show the 2 most recent events
		shown := 0
		for i := len(events.Items) - 1; i >= 0 && shown < 2; i-- {
			evt := events.Items[i]
			isWarning := evt.Type == corev1.EventTypeWarning ||
				evt.Reason == "HealthCheckFailed" ||
				evt.Reason == "DependencyNotReady"
			output.PrintEvent(evt.Reason, evt.Message, isWarning)
			shown++
		}
	} else {
		m.mu.Unlock()
	}
}

func (m *Monitor) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Determine the GVR for the resource
	var gvr schema.GroupVersionResource
	switch m.kind {
	case "kustomization":
		gvr = schema.GroupVersionResource{
			Group:    "kustomize.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "kustomizations",
		}
	case "helmrelease":
		gvr = schema.GroupVersionResource{
			Group:    "helm.toolkit.fluxcd.io",
			Version:  "v2beta1",
			Resource: "helmreleases",
		}
	case "source", "gitrepository":
		gvr = schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "gitrepositories",
		}
	default:
		return fmt.Errorf("unsupported resource kind: %s", m.kind)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %s reconciliation", m.kind)
			}

			// Check if resource is ready using dynamic client
			ready, err := m.checkResourceReady(gvr)
			if err != nil {
				// Continue waiting if we can't check status
				continue
			}
			if ready {
				return nil
			}
		}
	}
}

func (m *Monitor) checkResourceReady(gvr schema.GroupVersionResource) (bool, error) {
	obj, err := m.dynamicClient.Resource(gvr).Namespace(m.namespace).Get(m.ctx, m.name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	// Check status.conditions for Ready condition
	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if !found || err != nil {
		return false, err
	}

	conditions, found, err := unstructured.NestedSlice(status, "conditions")
	if !found || err != nil {
		return false, err
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condMap, "type")
		condStatus, _, _ := unstructured.NestedString(condMap, "status")

		if condType == "Ready" && condStatus == "True" {
			return true, nil
		}
	}

	return false, nil
}

func (m *Monitor) Stop() {
	m.cancel()
}
