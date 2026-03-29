/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.
*/

package alloc

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

// ASNAllocator picks an unused ASN in the broker pool (from ConfigMap-driven bounds).
type ASNAllocator struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
	pools        *Pools
}

// NewASNAllocator creates an ASNAllocator.
func NewASNAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string, pools *Pools) *ASNAllocator {
	if pools == nil {
		pools = DefaultPools()
	}
	return &ASNAllocator{
		brokerClient: brokerClient,
		brokerNS:     brokerNS,
		clusterID:    clusterID,
		pools:        pools,
	}
}

// AllocateASN allocates an ASN for the cluster.
func (a *ASNAllocator) AllocateASN(ctx context.Context, cluster *skynetv1.Cluster) (int32, error) {
	if cluster.Spec.ASN != 0 {
		klog.V(2).Infof("ASN already allocated for cluster %s: %d", a.clusterID, cluster.Spec.ASN)
		return cluster.Spec.ASN, nil
	}

	allocatedASNs, err := a.getAllocatedASNs(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get allocated ASNs")
	}

	for asn := a.pools.ASNMin; asn <= a.pools.ASNMax; asn++ {
		if !allocatedASNs[asn] {
			klog.V(4).Infof("Found available ASN: %d", asn)
			klog.Infof("Allocated ASN %d for cluster %s", asn, a.clusterID)
			return asn, nil
		}
	}

	return 0, errors.New("no available ASN in configured range")
}

func (a *ASNAllocator) getAllocatedASNs(ctx context.Context) (map[int32]bool, error) {
	clusterList, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Clusters from broker")
	}

	allocatedASNs := make(map[int32]bool)
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
