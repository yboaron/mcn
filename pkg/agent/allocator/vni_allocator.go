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

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

const (
	// VNIMin is the minimum VNI for multi-cluster networks
	VNIMin = 5000
	// VNIMax is the maximum VNI for multi-cluster networks
	VNIMax = 10000
	// RouteTargetASN is the fixed ASN used for route targets
	RouteTargetASN = 65000
)

// VNIAllocator handles VNI allocation for MultiClusterNetworks
type VNIAllocator struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
}

// NewVNIAllocator creates a new VNIAllocator
func NewVNIAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string) *VNIAllocator {
	return &VNIAllocator{
		brokerClient: brokerClient,
		brokerNS:     brokerNS,
		clusterID:    clusterID,
	}
}

// AllocateVNI allocates a VNI for a MultiClusterNetwork using optimistic locking
// Returns the allocated VNI or error if allocation fails
func (a *VNIAllocator) AllocateVNI(ctx context.Context, mcn *skynetv1.MultiClusterNetwork) (uint32, error) {
	// Check if already allocated
	if mcn.Spec.VNI != 0 {
		klog.V(2).Infof("VNI already allocated for MultiClusterNetwork %s: %d", mcn.Name, mcn.Spec.VNI)
		return mcn.Spec.VNI, nil
	}

	// Get all allocated VNIs from broker
	allocatedVNIs, err := a.getAllocatedVNIs(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get allocated VNIs")
	}

	// Find next available VNI
	vni, err := a.findNextAvailableVNI(allocatedVNIs)
	if err != nil {
		return 0, errors.Wrap(err, "failed to find available VNI")
	}

	klog.Infof("Allocated VNI %d for MultiClusterNetwork %s", vni, mcn.Name)
	return vni, nil
}

// getAllocatedVNIs retrieves all allocated VNIs from the broker
func (a *VNIAllocator) getAllocatedVNIs(ctx context.Context) (map[uint32]bool, error) {
	mcnList, err := a.brokerClient.Resource(skynetv1.MultiClusterNetworkGVR).Namespace(a.brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list MultiClusterNetworks from broker")
	}

	allocatedVNIs := make(map[uint32]bool)
	for i := range mcnList.Items {
		mcn := &skynetv1.MultiClusterNetwork{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(mcnList.Items[i].Object, mcn); err != nil {
			klog.Errorf("Failed to convert MultiClusterNetwork: %v", err)
			continue
		}

		if mcn.Spec.VNI != 0 {
			allocatedVNIs[mcn.Spec.VNI] = true
		}
	}

	klog.V(4).Infof("Found %d allocated VNIs on broker", len(allocatedVNIs))
	return allocatedVNIs, nil
}

// findNextAvailableVNI finds the next available VNI from the configured range
func (a *VNIAllocator) findNextAvailableVNI(allocatedVNIs map[uint32]bool) (uint32, error) {
	// Iterate through the VNI range (5000-10000)
	for vni := uint32(VNIMin); vni <= VNIMax; vni++ {
		if !allocatedVNIs[vni] {
			klog.V(4).Infof("Found available VNI: %d", vni)
			return vni, nil
		}
	}

	return 0, errors.Errorf("no available VNI in range [%d-%d]", VNIMin, VNIMax)
}

// ValidateVNI validates a VNI is within the configured range
func ValidateVNI(vni uint32) error {
	if vni < VNIMin || vni > VNIMax {
		return errors.Errorf("VNI %d is not in valid range [%d-%d]", vni, VNIMin, VNIMax)
	}
	return nil
}

// GenerateRouteTarget generates the route target string for a VNI
// Format: "<ASN>:<VNI>" (e.g., "65000:5001")
func GenerateRouteTarget(vni uint32) string {
	return formatRouteTarget(RouteTargetASN, vni)
}

// formatRouteTarget formats ASN and VNI into route target string
func formatRouteTarget(asn, vni uint32) string {
	return fmt.Sprintf("%d:%d", asn, vni)
}
