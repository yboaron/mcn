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

package vtep

import (
	"context"
	"fmt"
	"net"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

var (
	// VtepGVR is the GroupVersionResource for VTEP
	VtepGVR = schema.GroupVersionResource{
		Group:    "k8s.ovn.org",
		Version:  "v1",
		Resource: "vteps",
	}
)

// VtepManager manages VTEP resources for remote clusters
type VtepManager struct {
	localClient  dynamic.Interface
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
	vtepCIDR     string
	vtepIPIndex  int
}

// Config holds configuration for VtepManager
type Config struct {
	LocalClient  dynamic.Interface
	BrokerClient dynamic.Interface
	BrokerNS     string
	ClusterID    string
	VtepCIDR     string
}

// NewVtepManager creates a new VtepManager
func NewVtepManager(config *Config) (*VtepManager, error) {
	if config.ClusterID == "" {
		return nil, errors.New("clusterID is required")
	}
	if config.LocalClient == nil {
		return nil, errors.New("localClient is required")
	}
	if config.VtepCIDR == "" {
		return nil, errors.New("vtepCIDR is required")
	}

	return &VtepManager{
		localClient:  config.LocalClient,
		brokerClient: config.BrokerClient,
		brokerNS:     config.BrokerNS,
		clusterID:    config.ClusterID,
		vtepCIDR:     config.VtepCIDR,
		vtepIPIndex:  1, // Start from .0.1
	}, nil
}

// ReconcileVteps creates/updates VTEP resources for remote clusters
func (m *VtepManager) ReconcileVteps(ctx context.Context, remoteClusters []*skynetv1.Cluster) error {
	klog.V(2).Infof("Reconciling VTEPs for %d remote clusters", len(remoteClusters))

	for _, remoteCluster := range remoteClusters {
		if err := m.reconcileVtepForCluster(ctx, remoteCluster); err != nil {
			klog.Errorf("Failed to reconcile VTEP for cluster %s: %v", remoteCluster.Spec.ClusterID, err)
			continue
		}
	}

	return nil
}

// reconcileVtepForCluster creates/updates a VTEP resource for a remote cluster
func (m *VtepManager) reconcileVtepForCluster(ctx context.Context, remoteCluster *skynetv1.Cluster) error {
	vtepName := fmt.Sprintf("skynet-%s", remoteCluster.Spec.ClusterID)

	// Build VTEP spec
	vtepSpec := map[string]interface{}{
		"name": vtepName,
		"endpoints": m.buildVtepEndpoints(remoteCluster),
	}

	vtep := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "Vtep",
			"metadata": map[string]interface{}{
				"name": vtepName,
				"labels": map[string]interface{}{
					"skynet.io/cluster":       remoteCluster.Spec.ClusterID,
					"skynet.io/managed-by":    "skynet-agent",
					"skynet.io/local-cluster": m.clusterID,
				},
			},
			"spec": vtepSpec,
		},
	}

	// Check if VTEP already exists
	existingVtep, err := m.localClient.Resource(VtepGVR).Get(ctx, vtepName, metav1.GetOptions{})
	if err == nil {
		// Update existing VTEP
		vtep.SetResourceVersion(existingVtep.GetResourceVersion())
		_, err = m.localClient.Resource(VtepGVR).Update(ctx, vtep, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update VTEP %s", vtepName)
		}
		klog.V(4).Infof("Updated VTEP %s for remote cluster %s", vtepName, remoteCluster.Spec.ClusterID)
	} else {
		// Create new VTEP
		_, err = m.localClient.Resource(VtepGVR).Create(ctx, vtep, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create VTEP %s", vtepName)
		}
		klog.Infof("Created VTEP %s for remote cluster %s", vtepName, remoteCluster.Spec.ClusterID)
	}

	return nil
}

// buildVtepEndpoints builds the endpoints list for a VTEP from cluster endpoint info
func (m *VtepManager) buildVtepEndpoints(remoteCluster *skynetv1.Cluster) []map[string]interface{} {
	endpoints := []map[string]interface{}{}

	for _, ep := range remoteCluster.Status.Endpoints {
		endpoint := map[string]interface{}{
			"ip":   ep.BgpPeerIP,
			"vtep": ep.VtepIP,
		}
		endpoints = append(endpoints, endpoint)
	}

	return endpoints
}

// AllocateVtepIP allocates the next available VTEP IP from the cluster's VTEP CIDR
func (m *VtepManager) AllocateVtepIP() (string, error) {
	ip, ipNet, err := net.ParseCIDR(m.vtepCIDR)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse VTEP CIDR")
	}

	// Convert IP to 4-byte representation
	ip4 := ip.To4()
	if ip4 == nil {
		return "", errors.New("VTEP CIDR must be IPv4")
	}

	// Calculate the next IP based on vtepIPIndex
	// For a /16 like 100.0.0.0/16, we want to allocate 100.0.0.1, 100.0.0.2, etc.
	nextIP := make(net.IP, len(ip4))
	copy(nextIP, ip4)

	// Add the index to the IP
	// For /16, we modify the last two octets
	index := m.vtepIPIndex
	nextIP[3] = byte(index & 0xFF)
	nextIP[2] = byte((index >> 8) & 0xFF)

	// Check if IP is within the CIDR range
	if !ipNet.Contains(nextIP) {
		return "", errors.New("VTEP IP pool exhausted")
	}

	m.vtepIPIndex++
	return nextIP.String(), nil
}

// DeleteVtep deletes a VTEP resource for a removed cluster
func (m *VtepManager) DeleteVtep(ctx context.Context, clusterID string) error {
	vtepName := fmt.Sprintf("skynet-%s", clusterID)

	err := m.localClient.Resource(VtepGVR).Delete(ctx, vtepName, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete VTEP %s", vtepName)
	}

	klog.Infof("Deleted VTEP %s for cluster %s", vtepName, clusterID)
	return nil
}

// ListVteps lists all VTEP resources managed by SkyNet
func (m *VtepManager) ListVteps(ctx context.Context) ([]runtime.Object, error) {
	vtepList, err := m.localClient.Resource(VtepGVR).List(ctx, metav1.ListOptions{
		LabelSelector: "skynet.io/managed-by=skynet-agent",
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list VTEPs")
	}

	var vteps []runtime.Object
	for i := range vtepList.Items {
		vteps = append(vteps, &vtepList.Items[i])
	}

	return vteps, nil
}
