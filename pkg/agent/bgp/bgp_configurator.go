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

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
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
		topology:     config.Topology,
	}, nil
}

// ReconcileBGPConfig creates/updates FRRConfiguration for BGP peering
func (c *BGPConfigurator) ReconcileBGPConfig(ctx context.Context, remoteClusters []*skynetv1.Cluster,
	multiClusterNetworks []*skynetv1.MultiClusterNetwork) error {
	klog.V(2).Infof("Reconciling BGP configuration for %d remote clusters", len(remoteClusters))

	configName := "skynet-bgp-config"

	// Build FRRConfiguration
	frrConfig := c.buildFRRConfiguration(configName, remoteClusters, multiClusterNetworks)

	frrNamespace := "frr-k8s-system"

	// Check if FRRConfiguration already exists
	existingConfig, err := c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Get(ctx, configName, metav1.GetOptions{})
	if err == nil {
		// Update existing configuration
		frrConfig.SetResourceVersion(existingConfig.GetResourceVersion())
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Update(ctx, frrConfig, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to update FRRConfiguration")
		}
		klog.V(4).Info("Updated FRRConfiguration")
	} else {
		// Create new configuration
		_, err = c.localClient.Resource(FRRConfigurationGVR).Namespace(frrNamespace).Create(ctx, frrConfig, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to create FRRConfiguration")
		}
		klog.Info("Created FRRConfiguration")
	}

	return nil
}

// buildFRRConfiguration builds the FRRConfiguration object
func (c *BGPConfigurator) buildFRRConfiguration(name string, remoteClusters []*skynetv1.Cluster,
	multiClusterNetworks []*skynetv1.MultiClusterNetwork) *unstructured.Unstructured {

	// Build BGP router config
	router := map[string]interface{}{
		"asn":       c.localASN,
		"neighbors": c.buildNeighbors(remoteClusters),
	}

	// Build spec - no nodeSelector means apply to all nodes (FRR-K8s behavior)
	spec := map[string]interface{}{
		"bgp": map[string]interface{}{
			"routers": []map[string]interface{}{router},
		},
	}

	// TODO: Add L2VPN EVPN address family when FRR-K8s supports it
	// See: https://github.com/metallb/frr-k8s/pull/372

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "frrk8s.metallb.io/v1beta1",
			"kind":       "FRRConfiguration",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "frr-k8s-system",  // FRR-K8s watches this namespace
				"labels": map[string]interface{}{
					"skynet.io/managed-by":    "skynet-agent",
					"skynet.io/local-cluster": c.clusterID,
				},
			},
			"spec": spec,
		},
	}
}

// buildNeighbors builds the BGP neighbor list
func (c *BGPConfigurator) buildNeighbors(remoteClusters []*skynetv1.Cluster) []map[string]interface{} {
	var neighbors []map[string]interface{}

	for _, remoteCluster := range remoteClusters {
		for _, endpoint := range remoteCluster.Status.Endpoints {
			// Skip non-route-reflector nodes if using RR topology and this is not an RR
			if c.topology == TopologyRouteReflector && !endpoint.RouteReflector {
				continue
			}

			neighbor := map[string]interface{}{
				"address":      endpoint.BgpPeerIP,
				"asn":          remoteCluster.Spec.ASN,
				"ebgpMultiHop": true,
			}

			neighbors = append(neighbors, neighbor)
		}
	}

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

// buildNodeFRRConfiguration builds a node-specific FRRConfiguration
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
			"kubernetes.io/hostname": nodeName,
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
