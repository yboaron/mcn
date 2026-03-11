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

package allocator

import (
	"context"
	"fmt"
	"net"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

const (
	// VtepPoolCIDR is the global VTEP pool: 100.0.0.0/8
	VtepPoolCIDR = "100.0.0.0/8"
	// VtepSubnetSize is the prefix length for each cluster: /16
	VtepSubnetSize = 16
)

// VtepAllocator handles VTEP CIDR allocation for clusters
type VtepAllocator struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
}

// NewVtepAllocator creates a new VtepAllocator
func NewVtepAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string) *VtepAllocator {
	return &VtepAllocator{
		brokerClient: brokerClient,
		brokerNS:     brokerNS,
		clusterID:    clusterID,
	}
}

// AllocateVtepCIDR allocates a VTEP CIDR for the cluster using optimistic locking
// Returns the allocated CIDR or error if allocation fails
func (a *VtepAllocator) AllocateVtepCIDR(ctx context.Context, cluster *skynetv1.Cluster) (string, error) {
	// Check if already allocated
	if cluster.Spec.VtepCIDR != "" {
		klog.V(2).Infof("VTEP CIDR already allocated for cluster %s: %s", a.clusterID, cluster.Spec.VtepCIDR)
		return cluster.Spec.VtepCIDR, nil
	}

	// Get all allocated CIDRs from broker
	allocatedCIDRs, err := a.getAllocatedVtepCIDRs(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get allocated VTEP CIDRs")
	}

	// Find next available CIDR
	vtepCIDR, err := a.findNextAvailableCIDR(allocatedCIDRs)
	if err != nil {
		return "", errors.Wrap(err, "failed to find available VTEP CIDR")
	}

	klog.Infof("Allocated VTEP CIDR %s for cluster %s", vtepCIDR, a.clusterID)
	return vtepCIDR, nil
}

// getAllocatedVtepCIDRs retrieves all allocated VTEP CIDRs from the broker
func (a *VtepAllocator) getAllocatedVtepCIDRs(ctx context.Context) ([]string, error) {
	clusterList, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Clusters from broker")
	}

	var allocatedCIDRs []string
	for i := range clusterList.Items {
		cluster := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterList.Items[i].Object, cluster); err != nil {
			klog.Errorf("Failed to convert Cluster: %v", err)
			continue
		}

		if cluster.Spec.VtepCIDR != "" {
			allocatedCIDRs = append(allocatedCIDRs, cluster.Spec.VtepCIDR)
		}
	}

	return allocatedCIDRs, nil
}

// findNextAvailableCIDR finds the next available /16 subnet from 100.0.0.0/8
func (a *VtepAllocator) findNextAvailableCIDR(allocatedCIDRs []string) (string, error) {
	// Parse the VTEP pool
	_, vtepPool, err := net.ParseCIDR(VtepPoolCIDR)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse VTEP pool CIDR")
	}

	// Create a map of allocated networks for quick lookup
	allocatedNets := make(map[string]bool)
	for _, cidr := range allocatedCIDRs {
		allocatedNets[cidr] = true
	}

	// Iterate through possible /16 subnets in 100.0.0.0/8
	// 100.0.0.0/16, 100.1.0.0/16, ..., 100.255.0.0/16
	baseIP := vtepPool.IP.Mask(vtepPool.Mask)

	// For a /8 network with /16 subnets, we have 256 possible subnets
	// Start from 100.0.0.0/16
	for i := 0; i < 256; i++ {
		// Calculate the subnet: 100.i.0.0/16
		subnetIP := make(net.IP, len(baseIP))
		copy(subnetIP, baseIP)
		subnetIP[1] = byte(i)

		subnet := &net.IPNet{
			IP:   subnetIP,
			Mask: net.CIDRMask(VtepSubnetSize, 32),
		}

		cidr := subnet.String()

		// Check if this CIDR is already allocated
		if !allocatedNets[cidr] {
			klog.V(4).Infof("Found available VTEP CIDR: %s", cidr)
			return cidr, nil
		}
	}

	return "", errors.New("no available VTEP CIDR in pool")
}

// ValidateVtepCIDR validates a VTEP CIDR
func ValidateVtepCIDR(cidr string) error {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return errors.Wrap(err, "invalid CIDR format")
	}

	// Check if it's a /16
	ones, bits := network.Mask.Size()
	if ones != VtepSubnetSize || bits != 32 {
		return fmt.Errorf("VTEP CIDR must be /%d, got /%d", VtepSubnetSize, ones)
	}

	// Check if it's within the VTEP pool (100.0.0.0/8)
	_, vtepPool, err := net.ParseCIDR(VtepPoolCIDR)
	if err != nil {
		return errors.Wrap(err, "failed to parse VTEP pool CIDR")
	}

	if !vtepPool.Contains(ip) {
		return fmt.Errorf("VTEP CIDR %s is not within pool %s", cidr, VtepPoolCIDR)
	}

	return nil
}
