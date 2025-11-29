package events

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	"github.com/junovy-hosting/flux-enhanced-cli/pkg/output"
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

	// Check for KUBECONFIG environment variable
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}

	// Fall back to kubeconfig file
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
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
	startTime := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	statusTicker := time.NewTicker(10 * time.Second) // Show status every 10 seconds
	defer ticker.Stop()
	defer statusTicker.Stop()

	// Determine the GVR for the resource
	gvr, err := m.getResourceGVR()
	if err != nil {
		return err
	}

	lastStatusTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-statusTicker.C:
			// Show periodic status updates
			elapsed := time.Since(startTime)
			remaining := time.Until(deadline)
			status, conditions := m.getResourceStatus(gvr)
			if status != "" {
				output.PrintStatus(fmt.Sprintf("Still waiting... (elapsed: %s, remaining: %s)",
					formatDuration(elapsed), formatDuration(remaining)))
				if conditions != "" {
					output.PrintStatus(fmt.Sprintf("Current status: %s", conditions))
				}
			}
		case <-ticker.C:
			if time.Now().After(deadline) {
				// Show final status before timeout
				_, conditions := m.getResourceStatus(gvr)
				if conditions != "" {
					output.PrintStatus(fmt.Sprintf("Timeout reached. Last known status: %s", conditions))
				}
				return fmt.Errorf("timeout waiting for %s reconciliation", m.kind)
			}

			// Check if resource is ready using dynamic client
			ready, err := m.checkResourceReady(gvr)
			if err != nil {
				// Show error periodically but continue waiting
				if time.Since(lastStatusTime) > 10*time.Second {
					output.PrintStatus(fmt.Sprintf("Unable to check status: %v (will retry)", err))
					lastStatusTime = time.Now()
				}
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

func (m *Monitor) getResourceStatus(gvr schema.GroupVersionResource) (string, string) {
	obj, err := m.dynamicClient.Resource(gvr).Namespace(m.namespace).Get(m.ctx, m.name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Sprintf("error getting resource: %v", err)
	}

	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if !found || err != nil {
		return "unknown", ""
	}

	conditions, found, err := unstructured.NestedSlice(status, "conditions")
	if !found || err != nil {
		return "no conditions", ""
	}

	// Collect all condition statuses
	var statusParts []string
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condMap, "type")
		condStatus, _, _ := unstructured.NestedString(condMap, "status")
		condMessage, _, _ := unstructured.NestedString(condMap, "message")

		if condType == "Ready" {
			if condStatus == "True" {
				return "ready", "Ready=True"
			}
			if condMessage != "" {
				statusParts = append(statusParts, fmt.Sprintf("%s=%s (%s)", condType, condStatus, condMessage))
			} else {
				statusParts = append(statusParts, fmt.Sprintf("%s=%s", condType, condStatus))
			}
		} else if condStatus == "False" && condMessage != "" {
			// Show other failed conditions
			statusParts = append(statusParts, fmt.Sprintf("%s=%s: %s", condType, condStatus, condMessage))
		}
	}

	if len(statusParts) == 0 {
		return "checking", ""
	}

	return "not ready", strings.Join(statusParts, ", ")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func (m *Monitor) Stop() {
	m.cancel()
}

// getResourceGVR determines the GroupVersionResource for the monitored resource.
// For HelmRelease, it tries v2 first and falls back to v2beta1.
func (m *Monitor) getResourceGVR() (schema.GroupVersionResource, error) {
	switch m.kind {
	case "kustomization":
		return schema.GroupVersionResource{
			Group:    "kustomize.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "kustomizations",
		}, nil
	case "helmrelease":
		// Try v2 first (newer), fall back to v2beta1 (deprecated)
		gvrV2 := schema.GroupVersionResource{
			Group:    "helm.toolkit.fluxcd.io",
			Version:  "v2",
			Resource: "helmreleases",
		}
		// Test if v2 works by trying to get the resource
		_, err := m.dynamicClient.Resource(gvrV2).Namespace(m.namespace).Get(m.ctx, m.name, metav1.GetOptions{})
		if err == nil {
			return gvrV2, nil
		}
		// Fall back to v2beta1
		return schema.GroupVersionResource{
			Group:    "helm.toolkit.fluxcd.io",
			Version:  "v2beta1",
			Resource: "helmreleases",
		}, nil
	case "git", "gitrepository":
		return schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "gitrepositories",
		}, nil
	case "oci", "ocirepository":
		return schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1beta2",
			Resource: "ocirepositories",
		}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unsupported resource kind: %s", m.kind)
	}
}
