/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package endpoint

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

// EndpointReporter collects node information and reports endpoints
type EndpointReporter struct {
	k8sClient         kubernetes.Interface
	clusterID         string
	routeReflectorSel string
}

// Config holds configuration for EndpointReporter
type Config struct {
	K8sClient             kubernetes.Interface
	ClusterID             string
	RouteReflectorSelector string // Label selector for route reflector nodes
}

// NewEndpointReporter creates a new EndpointReporter
func NewEndpointReporter(config *Config) (*EndpointReporter, error) {
	if config.K8sClient == nil {
		return nil, errors.New("k8sClient is required")
	}
	if config.ClusterID == "" {
		return nil, errors.New("clusterID is required")
	}

	return &EndpointReporter{
		k8sClient:         config.K8sClient,
		clusterID:         config.ClusterID,
		routeReflectorSel: config.RouteReflectorSelector,
	}, nil
}

// CollectEndpoints collects node endpoint information from the cluster
// Phase 1: Only collects BGP peer IPs (node internal IPs)
// Phase 2: Will read VTEP IPs from VTEP CR status when OVN-K VTEP controller is available
// See: https://github.com/ovn-kubernetes/ovn-kubernetes/pull/6078
func (r *EndpointReporter) CollectEndpoints(ctx context.Context) ([]skynetv1.NodeEndpoint, error) {
	klog.V(2).Info("Collecting node endpoints")

	// List all nodes
	nodes, err := r.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list nodes")
	}

	var endpoints []skynetv1.NodeEndpoint

	for i := range nodes.Items {
		node := &nodes.Items[i]

		// Skip nodes that are not ready
		if !isNodeReady(node) {
			klog.V(4).Infof("Skipping non-ready node %s", node.Name)
			continue
		}

		// Get BGP peer IP (node's internal IP)
		bgpPeerIP := getNodeInternalIP(node)
		if bgpPeerIP == "" {
			klog.Warningf("Node %s has no internal IP, skipping", node.Name)
			continue
		}

		// Check if node is a route reflector
		isRR := r.isRouteReflector(node)

		endpoint := skynetv1.NodeEndpoint{
			Node:           node.Name,
			BgpPeerIP:      bgpPeerIP,
			// VtepIPs: Will be populated in Phase 2 from VTEP CR status
			RouteReflector: isRR,
		}

		endpoints = append(endpoints, endpoint)
		klog.V(4).Infof("Collected endpoint for node %s: bgpPeerIP=%s, isRR=%v",
			node.Name, bgpPeerIP, isRR)
	}

	klog.Infof("Collected %d node endpoints", len(endpoints))
	return endpoints, nil
}

// isNodeReady checks if a node is ready
func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// getNodeInternalIP returns the internal IP of a node
func getNodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// isRouteReflector checks if a node is designated as a route reflector
func (r *EndpointReporter) isRouteReflector(node *corev1.Node) bool {
	if r.routeReflectorSel == "" {
		return false
	}

	// Parse simple key=value label selector
	// Format: "key=value" or "key=value,key2=value2"
	selectors := strings.Split(r.routeReflectorSel, ",")
	for _, sel := range selectors {
		parts := strings.Split(strings.TrimSpace(sel), "=")
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if nodeValue, exists := node.Labels[key]; exists && nodeValue == value {
			return true
		}
	}

	return false
}

// SelectRouteReflectors selects nodes to be route reflectors based on a count
func (r *EndpointReporter) SelectRouteReflectors(ctx context.Context, count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}

	nodes, err := r.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list nodes")
	}

	var readyNodes []string
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if isNodeReady(node) {
			readyNodes = append(readyNodes, node.Name)
		}
	}

	if len(readyNodes) < count {
		count = len(readyNodes)
	}

	// Select first 'count' nodes as route reflectors
	return readyNodes[:count], nil
}

// LabelRouteReflectors adds route reflector label to specified nodes
func (r *EndpointReporter) LabelRouteReflectors(ctx context.Context, nodeNames []string) error {
	for _, nodeName := range nodeNames {
		node, err := r.k8sClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to get node %s: %v", nodeName, err)
			continue
		}

		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}

		node.Labels["skynet.io/route-reflector"] = "true"

		_, err = r.k8sClient.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed to label node %s as route reflector: %v", nodeName, err)
			continue
		}

		klog.Infof("Labeled node %s as route reflector", nodeName)
	}

	return nil
}
