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

package bgp

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/yboaron/mcn/pkg/apis/skynet.io/v1"
)

var (
	// FRRConfigurationGVR is the GroupVersionResource for FRRConfiguration
	FRRConfigurationGVR = schema.GroupVersionResource{
		Group:    "frrk8s.metallb.io",
		Version:  "v1beta1",
		Resource: "frrconfigurations",
	}
)

// BGPConfigurator manages FRRConfiguration resources for BGP peering
type BGPConfigurator struct {
	localClient  dynamic.Interface
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
	localASN     int32
	vtepCIDR     string
	topology     BGPTopology
}

// BGPTopology represents the BGP peering topology
type BGPTopology string

const (
	// TopologyFullMesh represents full mesh BGP peering
	TopologyFullMesh BGPTopology = "FullMesh"
	// TopologyRouteReflector represents route reflector BGP peering
	TopologyRouteReflector BGPTopology = "RouteReflector"
)

// Config holds configuration for BGPConfigurator
type Config struct {
	LocalClient  dynamic.Interface
	BrokerClient dynamic.Interface
	BrokerNS     string
	ClusterID    string
	LocalASN     int32
	VtepCIDR     string
	Topology     BGPTopology
}

// NewBGPConfigurator creates a new BGPConfigurator
func NewBGPConfigurator(config *Config) (*BGPConfigurator, error) {
	if config.ClusterID == "" {
		return nil, errors.New("clusterID is required")
	}
	if config.LocalClient == nil {
		return nil, errors.New("localClient is required")
	}
	if config.LocalASN == 0 {
		return nil, errors.New("localASN is required")
	}

	return &BGPConfigurator{
		localClient:  config.LocalClient,
		brokerClient: config.BrokerClient,
		brokerNS:     config.BrokerNS,
		clusterID:    config.ClusterID,
		localASN:     config.LocalASN,
		vtepCIDR:     config.VtepCIDR,
		topology:     config.Topology,
	}, nil
}

// ReconcileBGPConfig creates/updates FRRConfiguration for BGP peering
// Creates one per-node FRRConfiguration for each local node
// Note: We don't use a generic cluster-wide config because FRR-K8s validates
// all configs together and rejects multiple router IDs for the same ASN
func (c *BGPConfigurator) ReconcileBGPConfig(ctx context.Context, localCluster *skynetv1.Cluster, remoteClusters []*skynetv1.Cluster,
	multiClusterNetworks []*skynetv1.MultiClusterNetwork) error {
	klog.V(2).Infof("Reconciling BGP configuration: %d local endpoints, %d remote clusters",
		len(localCluster.Status.Endpoints), len(remoteClusters))

	// Delete old generic config if it exists (migration from old approach)
	if err := c.deleteGenericBGPConfig(ctx); err != nil {
		klog.V(4).Infof("No generic config to delete (expected): %v", err)
	}

	// Create per-node FRRConfiguration for each local node (ASN, router ID, neighbors excluding self)
	for _, endpoint := range localCluster.Status.Endpoints {
		neighbors := c.buildNeighborsForNode(endpoint.BgpPeerIP, localCluster, remoteClusters)
		if err := c.reconcileNodeBGPConfigWithNeighbors(ctx, endpoint.Node, endpoint.BgpPeerIP, neighbors); err != nil {
			klog.Errorf("Failed to reconcile BGP config for node %s: %v", endpoint.Node, err)
			// Continue with other nodes instead of failing completely
		}
	}

	klog.V(2).Infof("BGP configuration reconciled: %d per-node configs", len(localCluster.Status.Endpoints))
	return nil
}

// reconcileNodeBGPConfigWithNeighbors creates/updates per-node FRRConfiguration with neighbors
func (c *BGPConfigurator) reconcileNodeBGPConfigWithNeighbors(ctx context.Context, nodeName, bgpPeerIP string, neighbors []map[string]interface{}) error {
	configName := fmt.Sprintf("skynet-node-%s", nodeName)
	frrNamespace := "frr-k8s-system"

	// Build node-specific FRRConfiguration with neighbors
	frrConfig := c.buildNodeFRRConfigurationWithNeighbors(configName, nodeName, bgpPeerIP, neighbors)

	// Check if FRRConfiguration already exists
	existingConfig, err := c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Get(ctx, configName, metav1.GetOptions{})
	if err == nil {
		// Update existing configuration
		frrConfig.SetResourceVersion(existingConfig.GetResourceVersion())
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Update(ctx, frrConfig, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update node FRRConfiguration for %s", nodeName)
		}
		klog.V(4).Infof("Updated node FRRConfiguration for %s with %d neighbors", nodeName, len(neighbors))
	} else {
		// Create new configuration
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Create(ctx, frrConfig, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create node FRRConfiguration for %s", nodeName)
		}
		klog.Infof("Created node FRRConfiguration for %s with %d neighbors", nodeName, len(neighbors))
	}

	return nil
}

// deleteGenericBGPConfig deletes the old generic cluster-wide FRRConfiguration
// Used for migration from hybrid approach to per-node-only approach
func (c *BGPConfigurator) deleteGenericBGPConfig(ctx context.Context) error {
	configName := "skynet-bgp-config"
	frrNamespace := "frr-k8s-system"

	err := c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Delete(ctx, configName, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to delete generic FRRConfiguration")
	}

	klog.Info("Deleted generic FRRConfiguration (migrated to per-node configs)")
	return nil
}


// buildNeighborsForNode builds the BGP neighbor list for a specific node
// Excludes self from the neighbor list (proper full mesh)
// Includes both intra-cluster (iBGP with other local nodes) and inter-cluster (eBGP with remote nodes)
func (c *BGPConfigurator) buildNeighborsForNode(nodeBgpPeerIP string, localCluster *skynetv1.Cluster, remoteClusters []*skynetv1.Cluster) []map[string]interface{} {
	var neighbors []map[string]interface{}
	intraCount := 0
	interCount := 0

	// Add intra-cluster neighbors (iBGP peering within same cluster)
	// All local nodes share the same ASN, so this is iBGP full mesh
	for _, endpoint := range localCluster.Status.Endpoints {
		// Exclude self - don't peer with own IP
		if endpoint.BgpPeerIP == nodeBgpPeerIP {
			continue
		}

		neighbor := map[string]interface{}{
			"address": endpoint.BgpPeerIP,
			"asn":     localCluster.Spec.ASN, // Same ASN = iBGP
			// Don't specify toAdvertise/toReceive - FRR-K8s will allow all routes by default
		}

		neighbors = append(neighbors, neighbor)
		intraCount++
		klog.V(4).Infof("Node %s: added intra-cluster iBGP neighbor %s (ASN %d)", nodeBgpPeerIP, endpoint.BgpPeerIP, localCluster.Spec.ASN)
	}

	// Add inter-cluster neighbors (eBGP peering with remote clusters)
	// Remote clusters have different ASNs, so this is eBGP
	for _, remoteCluster := range remoteClusters {
		for _, endpoint := range remoteCluster.Status.Endpoints {
			// Skip non-route-reflector nodes if using RR topology and this is not an RR
			if c.topology == TopologyRouteReflector && !endpoint.RouteReflector {
				continue
			}

			neighbor := map[string]interface{}{
				"address":      endpoint.BgpPeerIP,
				"asn":          remoteCluster.Spec.ASN, // Different ASN = eBGP
				"ebgpMultiHop": true,
				// Don't specify toAdvertise/toReceive - FRR-K8s will allow all routes by default
			}

			neighbors = append(neighbors, neighbor)
			interCount++
			klog.V(4).Infof("Node %s: added inter-cluster eBGP neighbor %s (ASN %d)", nodeBgpPeerIP, endpoint.BgpPeerIP, remoteCluster.Spec.ASN)
		}
	}

	klog.V(2).Infof("Built neighbor list for node %s: %d intra-cluster (iBGP) + %d inter-cluster (eBGP) = %d total neighbors",
		nodeBgpPeerIP, intraCount, interCount, len(neighbors))

	return neighbors
}

// buildEVPNConfig builds raw FRR configuration for L2VPN EVPN address family
// This enables VTEP IP advertisement (underlay) between clusters
// TODO: Re-enable when FRR-K8s EVPN support is merged (https://github.com/metallb/frr-k8s/pull/372)
/*
func (c *BGPConfigurator) buildEVPNConfig(remoteClusters []*skynetv1.Cluster) string {
	var config string

	config = fmt.Sprintf("router bgp %d\n", c.localASN)
	config += "  address-family l2vpn evpn\n"

	// Activate EVPN for all remote cluster neighbors
	for _, remoteCluster := range remoteClusters {
		for _, endpoint := range remoteCluster.Status.Endpoints {
			// Skip non-route-reflector nodes if using RR topology and this is not an RR
			if c.topology == TopologyRouteReflector && !endpoint.RouteReflector {
				continue
			}

			config += fmt.Sprintf("    neighbor %s activate\n", endpoint.BgpPeerIP)
		}
	}

	config += "  exit-address-family\n"

	return config
}
*/

// ReconcileNodeBGPConfig creates/updates per-node FRRConfiguration for local BGP settings
func (c *BGPConfigurator) ReconcileNodeBGPConfig(ctx context.Context, nodeName, bgpPeerIP string) error {
	configName := fmt.Sprintf("skynet-node-%s", nodeName)

	// Build node-specific FRRConfiguration
	frrConfig := c.buildNodeFRRConfiguration(configName, nodeName, bgpPeerIP)

	// Check if FRRConfiguration already exists
	existingConfig, err := c.localClient.Resource(FRRConfigurationGVR).Namespace(metav1.NamespaceDefault).Get(ctx, configName, metav1.GetOptions{})
	if err == nil {
		// Update existing configuration
		frrConfig.SetResourceVersion(existingConfig.GetResourceVersion())
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(metav1.NamespaceDefault).Update(ctx, frrConfig, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update node FRRConfiguration for %s", nodeName)
		}
		klog.V(4).Infof("Updated node FRRConfiguration for %s", nodeName)
	} else {
		// Create new configuration
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(metav1.NamespaceDefault).Create(ctx, frrConfig, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create node FRRConfiguration for %s", nodeName)
		}
		klog.Infof("Created node FRRConfiguration for %s", nodeName)
	}

	return nil
}

// buildNodeFRRConfigurationWithNeighbors builds a node-specific FRRConfiguration with neighbors
// Note: We don't set router ID explicitly - FRR will auto-assign it to the node's IP
// This avoids FRR-K8s validation webhook errors about different router IDs in the same namespace
func (c *BGPConfigurator) buildNodeFRRConfigurationWithNeighbors(name, nodeName, bgpPeerIP string, neighbors []map[string]interface{}) *unstructured.Unstructured {
	router := map[string]interface{}{
		"asn":       c.localASN,
		// Don't set "id" - FRR will use node's IP as router ID automatically
		"neighbors": neighbors,
	}

	// Don't add prefixes - we want to advertise individual /32 VTEP IPs via redistribute connected,
	// not the /16 supernet via network statement

	spec := map[string]interface{}{
		"bgp": map[string]interface{}{
			"routers": []map[string]interface{}{router},
		},
		"nodeSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"kubernetes.io/hostname": nodeName,
			},
		},
	}

	// Advertise VTEP IPs via BGP by redistributing connected routes
	// This advertises the /32 loopback IPs (100.x.x.x/32) to remote clusters
	// so they learn how to reach our VTEP IPs for VXLAN encapsulation
	if c.vtepCIDR != "" {
		// Build raw config to redistribute connected and remove FRR-K8s default deny-all route-maps
		rawConfig := fmt.Sprintf(`router bgp %d
 address-family ipv4 unicast
  redistribute connected route-map VTEP_LOOPBACK
 exit-address-family
exit
!
ip prefix-list VTEP_PREFIXES permit %s ge 32 le 32
!
route-map VTEP_LOOPBACK permit 10
 match ip address prefix-list VTEP_PREFIXES
exit
!`, c.localASN, c.vtepCIDR)

		// Remove FRR-K8s generated route-maps that block advertisements
		// FRR-K8s creates <neighbor-ip>-out route-maps with "deny any" by default
		// We need to remove them so our redistributed /32 VTEP IPs can be advertised
		rawConfig += fmt.Sprintf("\nrouter bgp %d\n address-family ipv4 unicast\n", c.localASN)
		for _, n := range neighbors {
			neighborAddr := n["address"].(string)
			rawConfig += fmt.Sprintf("  no neighbor %s route-map %s-out out\n", neighborAddr, neighborAddr)
			rawConfig += fmt.Sprintf("  no neighbor %s route-map %s-in in\n", neighborAddr, neighborAddr)
		}
		rawConfig += " exit-address-family\nexit\n!"

		spec["raw"] = map[string]interface{}{
			"priority":  5, // Lower than OVN-K's EVPN config (priority 10)
			"rawConfig": rawConfig,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "frrk8s.metallb.io/v1beta1",
			"kind":       "FRRConfiguration",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "frr-k8s-system",
				"labels": map[string]interface{}{
					"skynet.io/managed-by":    "skynet-agent",
					"skynet.io/local-cluster": c.clusterID,
					"skynet.io/config-type":   "per-node",
					"skynet.io/node":          nodeName,
				},
			},
			"spec": spec,
		},
	}
}

// buildNodeFRRConfiguration builds a node-specific FRRConfiguration (legacy, for backwards compat)
func (c *BGPConfigurator) buildNodeFRRConfiguration(name, nodeName, bgpPeerIP string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"bgp": map[string]interface{}{
			"routers": []map[string]interface{}{
				{
					"asn": c.localASN,
					"id":  bgpPeerIP,
				},
			},
		},
		"nodeSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"kubernetes.io/hostname": nodeName,
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "frrk8s.metallb.io/v1beta1",
			"kind":       "FRRConfiguration",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": metav1.NamespaceDefault,
				"labels": map[string]interface{}{
					"skynet.io/managed-by":    "skynet-agent",
					"skynet.io/local-cluster": c.clusterID,
					"skynet.io/node":          nodeName,
				},
			},
			"spec": spec,
		},
	}
}

// DeleteBGPConfig deletes the FRRConfiguration
func (c *BGPConfigurator) DeleteBGPConfig(ctx context.Context) error {
	configName := "skynet-bgp-config"
	frrNamespace := "frr-k8s-system"

	err := c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Delete(ctx, configName, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to delete FRRConfiguration")
	}

	klog.Info("Deleted FRRConfiguration")
	return nil
}
