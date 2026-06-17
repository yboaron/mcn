// SPDX-FileCopyrightText: Copyright The SkyNet Contributors
// SPDX-License-Identifier: Apache-2.0

package routeadv

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/yboaron/mcn/pkg/apis/skynet.io/v1"
)

// Creator handles creating RouteAdvertisement CRs for EVPN-enabled CUDNs
type Creator struct {
	dynamicClient dynamic.Interface
	clusterName   string // Used for generating unique RouteAdvertisement names
}

// Config contains configuration for RouteAdvertisement Creator
type Config struct {
	DynamicClient dynamic.Interface
	ClusterName   string
}

// NewCreator creates a new RouteAdvertisement creator
func NewCreator(config *Config) (*Creator, error) {
	if config.DynamicClient == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	if config.ClusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}

	return &Creator{
		dynamicClient: config.DynamicClient,
		clusterName:   config.ClusterName,
	}, nil
}

var (
	routeAdvGVR = schema.GroupVersionResource{
		Group:    "k8s.ovn.org",
		Version:  "v1",
		Resource: "routeadvertisements",
	}
)

// CreateForCUDN creates a RouteAdvertisement CR for an EVPN-enabled CUDN
// This triggers OVN-K to generate FRRConfiguration for BGP route advertisement
func (c *Creator) CreateForCUDN(ctx context.Context, cudnName string, mcn *skynetv1.MultiClusterNetwork) error {
	raName := fmt.Sprintf("skynet-%s-%s", c.clusterName, cudnName)
	klog.Infof("Creating RouteAdvertisement %s for CUDN %s", raName, cudnName)

	// Check if RouteAdvertisement already exists
	existing, err := c.dynamicClient.Resource(routeAdvGVR).Get(ctx, raName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing RouteAdvertisement: %w", err)
	}

	if existing != nil {
		klog.V(4).Infof("RouteAdvertisement %s already exists, skipping creation", raName)
		return nil
	}

	// Build RouteAdvertisement spec
	ra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "RouteAdvertisements",
			"metadata": map[string]interface{}{
				"name": raName,
				"labels": map[string]interface{}{
					"skynet.io/cluster":    c.clusterName,
					"skynet.io/cudn":       cudnName,
					"skynet.io/mcn":        mcn.Name,
					"skynet.io/managed-by": "skynet-agent",
				},
			},
			"spec": map[string]interface{}{
				// Select all CUDNs (empty selector matches all)
				// TODO: Add labels to CUDN during creation to enable specific selection
				"networkSelectors": []interface{}{
					map[string]interface{}{
						"networkSelectionType": "ClusterUserDefinedNetworks",
						"clusterUserDefinedNetworkSelector": map[string]interface{}{
							"networkSelector": map[string]interface{}{},
						},
					},
				},
				// Select all nodes (empty nodeSelector matches all)
				"nodeSelector": map[string]interface{}{},
				// Select FRRConfiguration created by SkyNet agent
				"frrConfigurationSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"skynet.io/managed-by": "skynet-agent",
					},
				},
				// Advertise pod network routes
				"advertisements": []interface{}{
					"PodNetwork",
				},
				// For EVPN CUDNs, let OVN-K auto-select the VRF
				"targetVRF": "auto",
			},
		},
	}

	// Create the RouteAdvertisement
	_, err = c.dynamicClient.Resource(routeAdvGVR).Create(ctx, ra, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(4).Infof("RouteAdvertisement %s already exists (race condition)", raName)
			return nil
		}
		return fmt.Errorf("failed to create RouteAdvertisement: %w", err)
	}

	klog.Infof("Successfully created RouteAdvertisement %s for CUDN %s", raName, cudnName)
	klog.V(4).Infof("  RouteAdvertisement will trigger OVN-K to generate FRRConfiguration")
	klog.V(4).Infof("  Network: CUDN %s (VNI=%d, RT=%s)", cudnName, mcn.Spec.VNI, mcn.Spec.RouteTarget)
	return nil
}

// DeleteForCUDN removes the RouteAdvertisement for a CUDN (cleanup on disconnect)
func (c *Creator) DeleteForCUDN(ctx context.Context, cudnName string) error {
	raName := fmt.Sprintf("skynet-%s-%s", c.clusterName, cudnName)
	klog.Infof("Deleting RouteAdvertisement %s for CUDN %s", raName, cudnName)

	err := c.dynamicClient.Resource(routeAdvGVR).Delete(ctx, raName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("RouteAdvertisement %s not found, nothing to clean up", raName)
			return nil
		}
		return fmt.Errorf("failed to delete RouteAdvertisement: %w", err)
	}

	klog.Infof("Successfully deleted RouteAdvertisement %s", raName)
	return nil
}

// CreateForDefaultNetwork creates a RouteAdvertisement CR for the default pod network
// This triggers OVN-K to advertise node pod CIDRs via BGP for cross-cluster connectivity
func (c *Creator) CreateForDefaultNetwork(ctx context.Context, mcncName string) error {
	raName := "default-network-pod-routes"
	klog.Infof("Creating RouteAdvertisement %s for default network (MCNC: %s)", raName, mcncName)

	// Check if RouteAdvertisement already exists
	existing, err := c.dynamicClient.Resource(routeAdvGVR).Get(ctx, raName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing RouteAdvertisement: %w", err)
	}

	if existing != nil {
		klog.V(4).Infof("RouteAdvertisement %s already exists, skipping creation", raName)
		return nil
	}

	// Build RouteAdvertisement spec for default network
	ra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "RouteAdvertisements",
			"metadata": map[string]interface{}{
				"name": raName,
				"labels": map[string]interface{}{
					"skynet.io/cluster":      c.clusterName,
					"skynet.io/network-type": "default",
					"skynet.io/managed-by":   "skynet-agent",
					"skynet.io/mcnc":         mcncName,
				},
			},
			"spec": map[string]interface{}{
				// Select the default Kubernetes network
				"networkSelectors": []interface{}{
					map[string]interface{}{
						"networkSelectionType": "DefaultNetwork",
					},
				},
				// Empty nodeSelector matches all nodes (required for PodNetwork advertisement)
				"nodeSelector": map[string]interface{}{},
				// Match FRRConfiguration created by MCN agent (with disableMP: true)
				"frrConfigurationSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"skynet.io/config-type": "per-node",
					},
				},
				// Advertise pod network routes (each node's pod CIDR)
				"advertisements": []interface{}{
					"PodNetwork",
				},
			},
		},
	}

	// Create the RouteAdvertisement
	_, err = c.dynamicClient.Resource(routeAdvGVR).Create(ctx, ra, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(4).Infof("RouteAdvertisement %s already exists (race condition)", raName)
			return nil
		}
		return fmt.Errorf("failed to create RouteAdvertisement: %w", err)
	}

	klog.Infof("Successfully created RouteAdvertisement %s for default network", raName)
	klog.V(4).Infof("  RouteAdvertisement will trigger OVN-K to advertise pod CIDRs via BGP")
	klog.V(4).Infof("  Each node will advertise its local pod CIDR to BGP neighbors")
	return nil
}

// DeleteForDefaultNetwork removes the RouteAdvertisement for default network (cleanup on disconnect)
func (c *Creator) DeleteForDefaultNetwork(ctx context.Context) error {
	raName := "default-network-pod-routes"
	klog.Infof("Deleting RouteAdvertisement %s for default network", raName)

	err := c.dynamicClient.Resource(routeAdvGVR).Delete(ctx, raName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("RouteAdvertisement %s not found, nothing to clean up", raName)
			return nil
		}
		return fmt.Errorf("failed to delete RouteAdvertisement: %w", err)
	}

	klog.Infof("Successfully deleted RouteAdvertisement %s", raName)
	return nil
}

// ListForCluster lists all RouteAdvertisements managed by this cluster's MCN agent
func (c *Creator) ListForCluster(ctx context.Context) ([]string, error) {
	list, err := c.dynamicClient.Resource(routeAdvGVR).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("skynet.io/cluster=%s,skynet.io/managed-by=skynet-agent", c.clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list RouteAdvertisements: %w", err)
	}

	var names []string
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}

	return names, nil
}
