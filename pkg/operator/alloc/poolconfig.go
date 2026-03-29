/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Broker pool bounds (ASN range, VTEP supernet) live in a ConfigMap on the broker cluster
so they are not hard-coded in allocator logic.
*/

package alloc

import (
	"context"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BrokerPoolsConfigMap is the ConfigMap name in the broker namespace (skynet-broker by default).
const BrokerPoolsConfigMap = "skynet-broker-pools"

// Pools holds ASN, VTEP, and VNI allocation bounds loaded from the broker ConfigMap (with defaults if missing).
type Pools struct {
	ASNMin         int32
	ASNMax         int32
	VtepPoolCIDR   string
	VtepPrefixLen  int
	VNIMin         uint32
	VNIMax         uint32
	RouteTargetASN uint32
}

// DefaultPools returns RFC 6996–style private ASN bounds and the default VTEP supernet used when no ConfigMap exists.
func DefaultPools() *Pools {
	return &Pools{
		ASNMin:         64512,
		ASNMax:         65534,
		VtepPoolCIDR:   "100.0.0.0/8",
		VtepPrefixLen:  16,
		VNIMin:         5000,
		VNIMax:         10000,
		RouteTargetASN: 65000,
	}
}

// LoadPoolsFromConfigMap reads pool configuration from the broker. If the ConfigMap is absent, returns DefaultPools().
func LoadPoolsFromConfigMap(ctx context.Context, k8s kubernetes.Interface, brokerNS string) (*Pools, error) {
	cm, err := k8s.CoreV1().ConfigMaps(brokerNS).Get(ctx, BrokerPoolsConfigMap, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return DefaultPools(), nil
		}
		return nil, errors.Wrap(err, "get broker pools ConfigMap")
	}
	return poolsFromConfigMapData(cm.Data)
}

func poolsFromConfigMapData(data map[string]string) (*Pools, error) {
	p := DefaultPools()
	if data == nil {
		return p, nil
	}
	if v := strings.TrimSpace(data["asnMin"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "parse asnMin")
		}
		p.ASNMin = int32(n)
	}
	if v := strings.TrimSpace(data["asnMax"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "parse asnMax")
		}
		p.ASNMax = int32(n)
	}
	if v := strings.TrimSpace(data["vtepPoolCIDR"]); v != "" {
		p.VtepPoolCIDR = v
	}
	if v := strings.TrimSpace(data["vtepPrefixLen"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.Wrap(err, "parse vtepPrefixLen")
		}
		p.VtepPrefixLen = n
	}
	if v := strings.TrimSpace(data["vniMin"]); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "parse vniMin")
		}
		p.VNIMin = uint32(n)
	}
	if v := strings.TrimSpace(data["vniMax"]); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "parse vniMax")
		}
		p.VNIMax = uint32(n)
	}
	if v := strings.TrimSpace(data["routeTargetASN"]); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "parse routeTargetASN")
		}
		p.RouteTargetASN = uint32(n)
	}
	if p.ASNMin > p.ASNMax {
		return nil, errors.Errorf("invalid ASN range: asnMin %d > asnMax %d", p.ASNMin, p.ASNMax)
	}
	if p.VtepPrefixLen < 8 || p.VtepPrefixLen > 30 {
		return nil, errors.Errorf("invalid vtepPrefixLen: %d", p.VtepPrefixLen)
	}
	if p.VNIMin > p.VNIMax {
		return nil, errors.Errorf("invalid VNI range: vniMin %d > vniMax %d", p.VNIMin, p.VNIMax)
	}
	return p, nil
}

// PoolsFromValues builds Pools from operator-injected env (skynet-agent). Used when validating allocation without reading the broker API.
func PoolsFromValues(asnMin, asnMax int32, vtepPool string, vtepPrefix int) (*Pools, error) {
	p := &Pools{
		ASNMin:        asnMin,
		ASNMax:        asnMax,
		VtepPoolCIDR:  vtepPool,
		VtepPrefixLen: vtepPrefix,
	}
	if p.ASNMin > p.ASNMax {
		return nil, errors.Errorf("invalid ASN range: min %d > max %d", p.ASNMin, p.ASNMax)
	}
	if p.VtepPoolCIDR == "" {
		return nil, errors.New("vtep pool CIDR is empty")
	}
	if p.VtepPrefixLen < 8 || p.VtepPrefixLen > 30 {
		return nil, errors.Errorf("invalid vtep prefix length: %d", p.VtepPrefixLen)
	}
	return p, nil
}

// Example broker ConfigMap (YAML):
//
//	apiVersion: v1
//	kind: ConfigMap
//	metadata:
//	  name: skynet-broker-pools
//	  namespace: skynet-broker
//	data:
//	  asnMin: "64512"
//	  asnMax: "65534"
//	  vtepPoolCIDR: "100.0.0.0/8"
//	  vtepPrefixLen: "16"
//	  vniMin: "5000"
//	  vniMax: "10000"
//	  routeTargetASN: "65000"
