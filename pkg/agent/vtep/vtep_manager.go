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
	"net"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

var (
	// VtepGVR is the GroupVersionResource for VTEP
	VtepGVR = schema.GroupVersionResource{
		Group:    "k8s.ovn.org",
		Version:  "v1",
		Resource: "vteps",
	}
)

const (
	// LocalVTEPName is the name of the local VTEP resource
	LocalVTEPName = "skynet-local"
	// VTEPModeManaged means OVN-K manages VTEP IP allocation
	VTEPModeManaged = "Managed"
)

// VtepManager manages VTEP resources for the local cluster
// It creates a single VTEP CR that tells OVN-K to allocate VTEP IPs for local nodes
type VtepManager struct {
	localClient dynamic.Interface
	clusterID   string
	vtepCIDR    string
	vtepIPIndex int
}

// Config holds configuration for VtepManager
type Config struct {
	LocalClient dynamic.Interface
	ClusterID   string
	VtepCIDR    string
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
		localClient: config.LocalClient,
		clusterID:   config.ClusterID,
		vtepCIDR:    config.VtepCIDR,
		vtepIPIndex: 1, // Start from .0.1
	}, nil
}

// EnsureLocalVTEP creates or updates the local VTEP resource
// This configures OVN-K to allocate VTEP IPs for nodes in this cluster
func (m *VtepManager) EnsureLocalVTEP(ctx context.Context) error {
	klog.Infof("Ensuring local VTEP with CIDR %s", m.vtepCIDR)

	// Build VTEP according to OVN-K spec
	// See: go-controller/pkg/crd/vtep/v1/types.go
	vtep := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "VTEP",
			"metadata": map[string]interface{}{
				"name": LocalVTEPName,
				"labels": map[string]interface{}{
					"skynet.io/managed-by": "skynet-agent",
					"skynet.io/cluster":    m.clusterID,
				},
			},
			"spec": map[string]interface{}{
				// CIDRs is the list of IP ranges from which VTEP IPs are allocated
				// This tells OVN-K to allocate VTEP IPs from this CIDR
				"cidrs": []string{m.vtepCIDR},
				// Mode: "Managed" means OVN-K allocates and assigns VTEP IPs per node automatically
				"mode": VTEPModeManaged,
			},
		},
	}

	// Check if VTEP already exists
	existingVtep, err := m.localClient.Resource(VtepGVR).Get(ctx, LocalVTEPName, metav1.GetOptions{})
	if err == nil {
		// Update existing VTEP
		vtep.SetResourceVersion(existingVtep.GetResourceVersion())
		_, err = m.localClient.Resource(VtepGVR).Update(ctx, vtep, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to update local VTEP")
		}
		klog.V(2).Infof("Updated local VTEP with CIDR %s", m.vtepCIDR)
	} else if apierrors.IsNotFound(err) {
		// Create new VTEP
		_, err = m.localClient.Resource(VtepGVR).Create(ctx, vtep, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to create local VTEP")
		}
		klog.Infof("Created local VTEP with CIDR %s (mode: %s)", m.vtepCIDR, VTEPModeManaged)
	} else {
		return errors.Wrap(err, "failed to get local VTEP")
	}

	return nil
}

// AllocateVtepIP allocates a VTEP IP for a node from the cluster's VTEP CIDR
// This is used by the endpoint reporter when collecting node information
// Note: This is a temporary allocation for reporting to the broker.
// OVN-K will manage the actual VTEP IP allocation via the VTEP CR.
func (m *VtepManager) AllocateVtepIP(nodeName string) (string, error) {
	_, network, err := net.ParseCIDR(m.vtepCIDR)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse VTEP CIDR %s", m.vtepCIDR)
	}

	// Calculate IP from base network and index
	// For 100.0.0.0/16, we allocate 100.0.0.1, 100.0.0.2, etc.
	baseIP := network.IP
	ip := make(net.IP, len(baseIP))
	copy(ip, baseIP)

	// For /16 networks, increment the last octet
	// This works for up to 254 nodes per cluster
	ip[len(ip)-1] = byte(m.vtepIPIndex)

	vtepIP := ip.String()
	m.vtepIPIndex++

	klog.V(4).Infof("Allocated VTEP IP %s for node %s", vtepIP, nodeName)
	return vtepIP, nil
}

// DeleteLocalVTEP deletes the local VTEP resource
func (m *VtepManager) DeleteLocalVTEP(ctx context.Context) error {
	err := m.localClient.Resource(VtepGVR).Delete(ctx, LocalVTEPName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete local VTEP")
	}

	klog.Info("Deleted local VTEP")
	return nil
}
