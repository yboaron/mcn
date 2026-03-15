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

package mcn

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/agent/allocator"
)

const (
	// FinalizerDomain is the domain for SkyNet finalizers
	FinalizerDomain = "multicluster.ovn.org"
)

// MCNManager manages MultiClusterNetwork resources on the broker
type MCNManager struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
	vniAllocator *allocator.VNIAllocator
}

// Config holds configuration for MCNManager
type Config struct {
	BrokerClient dynamic.Interface
	BrokerNS     string
	ClusterID    string
	VNIAllocator *allocator.VNIAllocator
}

// NewMCNManager creates a new MCNManager
func NewMCNManager(config *Config) (*MCNManager, error) {
	if config.BrokerClient == nil {
		return nil, errors.New("brokerClient is required")
	}
	if config.BrokerNS == "" {
		return nil, errors.New("brokerNS is required")
	}
	if config.ClusterID == "" {
		return nil, errors.New("clusterID is required")
	}
	if config.VNIAllocator == nil {
		return nil, errors.New("vniAllocator is required")
	}

	return &MCNManager{
		brokerClient: config.BrokerClient,
		brokerNS:     config.BrokerNS,
		clusterID:    config.ClusterID,
		vniAllocator: config.VNIAllocator,
	}, nil
}

// CreateOrJoinMCN creates a new MultiClusterNetwork or joins an existing one
// Returns the MCN with VNI and RouteTarget populated
func (m *MCNManager) CreateOrJoinMCN(ctx context.Context, mcnName string, topology skynetv1.NetworkTopology) (*skynetv1.MultiClusterNetwork, error) {
	klog.V(2).Infof("CreateOrJoinMCN: attempting to join/create MCN %s for cluster %s", mcnName, m.clusterID)

	// Try to get existing MCN
	mcn, err := m.GetMCN(ctx, mcnName)
	if err == nil {
		// MCN exists - join it
		klog.Infof("MCN %s exists, joining with VNI %d", mcnName, mcn.Spec.VNI)
		return m.joinExistingMCN(ctx, mcn)
	}

	if !apierrors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to get MCN %s", mcnName)
	}

	// MCN doesn't exist - create it
	klog.Infof("MCN %s does not exist, creating new MCN", mcnName)
	return m.createNewMCN(ctx, mcnName, topology)
}

// joinExistingMCN adds this cluster's finalizer to an existing MCN
func (m *MCNManager) joinExistingMCN(ctx context.Context, mcn *skynetv1.MultiClusterNetwork) (*skynetv1.MultiClusterNetwork, error) {
	// Check if our finalizer already exists
	finalizer := m.getFinalizerName()
	if containsString(mcn.Finalizers, finalizer) {
		klog.V(2).Infof("Cluster %s already joined MCN %s", m.clusterID, mcn.Name)
		return mcn, nil
	}

	// Add our finalizer
	mcn.Finalizers = append(mcn.Finalizers, finalizer)

	// Update MCN on broker
	updatedMCN, err := m.updateMCN(ctx, mcn)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to add finalizer to MCN %s", mcn.Name)
	}

	klog.Infof("Successfully joined MCN %s (VNI: %d, RT: %s)", mcn.Name, mcn.Spec.VNI, mcn.Spec.RouteTarget)
	return updatedMCN, nil
}

// createNewMCN creates a new MultiClusterNetwork with allocated VNI
func (m *MCNManager) createNewMCN(ctx context.Context, mcnName string, topology skynetv1.NetworkTopology) (*skynetv1.MultiClusterNetwork, error) {
	// Create MCN object
	mcn := &skynetv1.MultiClusterNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:       mcnName,
			Finalizers: []string{m.getFinalizerName()},
		},
		Spec: skynetv1.MultiClusterNetworkSpec{
			Topology: topology,
		},
	}

	// Allocate VNI
	vni, err := m.vniAllocator.AllocateVNI(ctx, mcn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to allocate VNI")
	}

	// Generate route target
	routeTarget := allocator.GenerateRouteTarget(vni)

	// Populate spec
	mcn.Spec.VNI = vni
	mcn.Spec.RouteTarget = routeTarget

	// Convert to unstructured
	unstructuredMCN, err := runtime.DefaultUnstructuredConverter.ToUnstructured(mcn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert MCN to unstructured")
	}

	// Create on broker
	created, err := m.brokerClient.Resource(skynetv1.MultiClusterNetworkGVR).Namespace(m.brokerNS).Create(
		ctx,
		&unstructured.Unstructured{Object: unstructuredMCN},
		metav1.CreateOptions{},
	)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition - another cluster created it first
			// Try to join the existing one
			klog.V(2).Infof("MCN %s was created by another cluster, attempting to join", mcnName)
			existingMCN, getErr := m.GetMCN(ctx, mcnName)
			if getErr != nil {
				return nil, errors.Wrap(getErr, "failed to get MCN after create conflict")
			}
			return m.joinExistingMCN(ctx, existingMCN)
		}
		return nil, errors.Wrapf(err, "failed to create MCN %s on broker", mcnName)
	}

	// Convert back to typed object
	createdMCN := &skynetv1.MultiClusterNetwork{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(created.Object, createdMCN); err != nil {
		return nil, errors.Wrap(err, "failed to convert created MCN")
	}

	klog.Infof("Successfully created MCN %s (VNI: %d, RT: %s, Topology: %s)",
		mcnName, createdMCN.Spec.VNI, createdMCN.Spec.RouteTarget, createdMCN.Spec.Topology)

	return createdMCN, nil
}

// LeaveMCN removes this cluster's finalizer from the MCN
// If this is the last cluster, the MCN can be garbage collected
func (m *MCNManager) LeaveMCN(ctx context.Context, mcnName string) error {
	klog.V(2).Infof("LeaveMCN: removing cluster %s from MCN %s", m.clusterID, mcnName)

	// Get the MCN
	mcn, err := m.GetMCN(ctx, mcnName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(2).Infof("MCN %s not found, nothing to leave", mcnName)
			return nil
		}
		return errors.Wrapf(err, "failed to get MCN %s", mcnName)
	}

	// Remove our finalizer
	finalizer := m.getFinalizerName()
	if !containsString(mcn.Finalizers, finalizer) {
		klog.V(2).Infof("Cluster %s not joined to MCN %s, nothing to do", m.clusterID, mcnName)
		return nil
	}

	mcn.Finalizers = removeString(mcn.Finalizers, finalizer)

	// Update MCN on broker
	_, err = m.updateMCN(ctx, mcn)
	if err != nil {
		return errors.Wrapf(err, "failed to remove finalizer from MCN %s", mcnName)
	}

	klog.Infof("Successfully left MCN %s", mcnName)
	return nil
}

// GetMCN retrieves a MultiClusterNetwork from the broker
func (m *MCNManager) GetMCN(ctx context.Context, mcnName string) (*skynetv1.MultiClusterNetwork, error) {
	unstructuredMCN, err := m.brokerClient.Resource(skynetv1.MultiClusterNetworkGVR).Namespace(m.brokerNS).Get(
		ctx,
		mcnName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}

	mcn := &skynetv1.MultiClusterNetwork{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredMCN.Object, mcn); err != nil {
		return nil, errors.Wrap(err, "failed to convert MCN from unstructured")
	}

	return mcn, nil
}

// updateMCN updates a MultiClusterNetwork on the broker
func (m *MCNManager) updateMCN(ctx context.Context, mcn *skynetv1.MultiClusterNetwork) (*skynetv1.MultiClusterNetwork, error) {
	unstructuredMCN, err := runtime.DefaultUnstructuredConverter.ToUnstructured(mcn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert MCN to unstructured")
	}

	updated, err := m.brokerClient.Resource(skynetv1.MultiClusterNetworkGVR).Namespace(m.brokerNS).Update(
		ctx,
		&unstructured.Unstructured{Object: unstructuredMCN},
		metav1.UpdateOptions{},
	)
	if err != nil {
		return nil, err
	}

	updatedMCN := &skynetv1.MultiClusterNetwork{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(updated.Object, updatedMCN); err != nil {
		return nil, errors.Wrap(err, "failed to convert updated MCN")
	}

	return updatedMCN, nil
}

// getFinalizerName returns the finalizer name for this cluster
// Format: <clusterID>.multicluster.ovn.org
func (m *MCNManager) getFinalizerName() string {
	return fmt.Sprintf("%s.%s", m.clusterID, FinalizerDomain)
}

// Helper functions for slice operations

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
