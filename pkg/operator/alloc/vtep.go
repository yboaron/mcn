/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.
*/

package alloc

import (
	"context"
	"net"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

// VtepAllocator picks an unused /N subnet from the configured VTEP pool on the broker.
type VtepAllocator struct {
	brokerClient dynamic.Interface
	brokerNS     string
	clusterID    string
	pools        *Pools
}

// NewVtepAllocator creates a VtepAllocator.
func NewVtepAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string, pools *Pools) *VtepAllocator {
	if pools == nil {
		pools = DefaultPools()
	}
	return &VtepAllocator{
		brokerClient: brokerClient,
		brokerNS:     brokerNS,
		clusterID:    clusterID,
		pools:        pools,
	}
}

// AllocateVtepCIDR allocates a VTEP CIDR for the cluster.
// For the default pool 100.0.0.0/8 with prefix /16, allocation walks 100.x.0.0/16.
func (a *VtepAllocator) AllocateVtepCIDR(ctx context.Context, cluster *skynetv1.Cluster) (string, error) {
	if cluster.Spec.VtepCIDR != "" {
		klog.V(2).Infof("VTEP CIDR already allocated for cluster %s: %s", a.clusterID, cluster.Spec.VtepCIDR)
		return cluster.Spec.VtepCIDR, nil
	}

	allocatedCIDRs, err := a.getAllocatedVtepCIDRs(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get allocated VTEP CIDRs")
	}

	cidr, err := a.findNextAvailableCIDR(allocatedCIDRs)
	if err != nil {
		return "", errors.Wrap(err, "failed to find available VTEP CIDR")
	}

	klog.Infof("Allocated VTEP CIDR %s for cluster %s", cidr, a.clusterID)
	return cidr, nil
}

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

func (a *VtepAllocator) findNextAvailableCIDR(allocatedCIDRs []string) (string, error) {
	_, vtepPool, err := net.ParseCIDR(a.pools.VtepPoolCIDR)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse VTEP pool CIDR")
	}

	poolOnes, _ := vtepPool.Mask.Size()
	if poolOnes != 8 || a.pools.VtepPrefixLen != 16 {
		return "", errors.Errorf("VTEP allocator supports /8 pool with /16 subnets only (got pool /%d, prefix /%d)", poolOnes, a.pools.VtepPrefixLen)
	}

	allocatedNets := make(map[string]bool)
	for _, cidr := range allocatedCIDRs {
		allocatedNets[cidr] = true
	}

	baseIP := vtepPool.IP.Mask(vtepPool.Mask)
	for i := 0; i < 256; i++ {
		subnetIP := make(net.IP, len(baseIP))
		copy(subnetIP, baseIP)
		subnetIP[1] = byte(i)

		subnet := &net.IPNet{
			IP:   subnetIP,
			Mask: net.CIDRMask(a.pools.VtepPrefixLen, 32),
		}
		cidr := subnet.String()
		if !allocatedNets[cidr] {
			klog.V(4).Infof("Found available VTEP CIDR: %s", cidr)
			return cidr, nil
		}
	}
	return "", errors.New("no available VTEP CIDR in pool")
}
