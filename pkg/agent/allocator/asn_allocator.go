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

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

const (
	// ASNMin is the minimum private ASN (RFC 6996)
	ASNMin = 64512
	// ASNMax is the maximum private ASN (RFC 6996)
	ASNMax = 65534
)

// ASNAllocator handles ASN allocation for clusters
type ASNAllocator struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
}

// NewASNAllocator creates a new ASNAllocator
func NewASNAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string) *ASNAllocator {
	return &ASNAllocator{
		brokerClient: brokerClient,
		brokerNS:     brokerNS,
		clusterID:    clusterID,
	}
}

// AllocateASN allocates an ASN for the cluster using optimistic locking
// Returns the allocated ASN or error if allocation fails
func (a *ASNAllocator) AllocateASN(ctx context.Context, cluster *skynetv1.Cluster) (uint32, error) {
	// Check if already allocated
	if cluster.Spec.ASN != 0 {
		klog.V(2).Infof("ASN already allocated for cluster %s: %d", a.clusterID, cluster.Spec.ASN)
		return cluster.Spec.ASN, nil
	}

	// Get all allocated ASNs from broker
	allocatedASNs, err := a.getAllocatedASNs(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get allocated ASNs")
	}

	// Find next available ASN
	asn, err := a.findNextAvailableASN(allocatedASNs)
	if err != nil {
		return 0, errors.Wrap(err, "failed to find available ASN")
	}

	klog.Infof("Allocated ASN %d for cluster %s", asn, a.clusterID)
	return asn, nil
}

// getAllocatedASNs retrieves all allocated ASNs from the broker
func (a *ASNAllocator) getAllocatedASNs(ctx context.Context) (map[uint32]bool, error) {
	clusterList, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Clusters from broker")
	}

	allocatedASNs := make(map[uint32]bool)
	for i := range clusterList.Items {
		cluster := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterList.Items[i].Object, cluster); err != nil {
			klog.Errorf("Failed to convert Cluster: %v", err)
			continue
		}

		if cluster.Spec.ASN != 0 {
			allocatedASNs[cluster.Spec.ASN] = true
		}
	}

	return allocatedASNs, nil
}

// findNextAvailableASN finds the next available ASN from the private ASN range
func (a *ASNAllocator) findNextAvailableASN(allocatedASNs map[uint32]bool) (uint32, error) {
	// Iterate through the private ASN range (64512-65534)
	for asn := uint32(ASNMin); asn <= ASNMax; asn++ {
		if !allocatedASNs[asn] {
			klog.V(4).Infof("Found available ASN: %d", asn)
			return asn, nil
		}
	}

	return 0, errors.New("no available ASN in private range")
}

// ValidateASN validates an ASN is within the private range
func ValidateASN(asn uint32) error {
	if asn < ASNMin || asn > ASNMax {
		return errors.Errorf("ASN %d is not in private range [%d-%d]", asn, ASNMin, ASNMax)
	}
	return nil
}
