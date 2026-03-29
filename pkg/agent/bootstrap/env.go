/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Environment variables for the skynet-agent process, typically injected by skynet-operator
from Skynet status (Submariner-style). Configuration is env-only; VTEP/ASN come from SKYNET_ALLOCATED_*.
*/

package bootstrap

import (
	"os"
	"strconv"
	"strings"
)

// Well-known environment variable names (operator contract).
const (
	EnvClusterID = "SKYNET_CLUSTER_ID"

	// Broker REST access (preferred when operator mounts/syncs broker credentials).
	EnvBrokerAPIServer   = "SKYNET_BROKER_API_SERVER"
	EnvBrokerToken       = "SKYNET_BROKER_TOKEN"
	EnvBrokerCA          = "SKYNET_BROKER_CA"          // PEM bytes as string; optional
	EnvBrokerNamespace   = "SKYNET_BROKER_NAMESPACE"    // broker-side namespace for Cluster CRs; default below
	EnvBrokerKubeconfig  = "SKYNET_BROKER_KUBECONFIG" // path to kubeconfig; dev / alternate

	// Mirrored from Skynet status by the operator into the agent pod (required).
	EnvAllocatedVtepCIDR = "SKYNET_ALLOCATED_VTEP_CIDR"
	EnvAllocatedASN      = "SKYNET_ALLOCATED_ASN"

	// Pool bounds from broker ConfigMap skynet-broker-pools (operator mirrors into the agent for validation).
	EnvPoolASNMin         = "SKYNET_POOL_ASN_MIN"
	EnvPoolASNMax         = "SKYNET_POOL_ASN_MAX"
	EnvPoolVtepCIDR       = "SKYNET_POOL_VTEP_CIDR"
	EnvPoolVtepPrefixLen  = "SKYNET_POOL_VTEP_PREFIX_LEN"
)

const DefaultBrokerNamespace = "skynet-broker"

// FromEnvironment reads operator-oriented configuration. Empty strings mean "not set".
type FromEnvironment struct {
	ClusterID            string
	BrokerAPIServer      string
	BrokerToken          string
	BrokerCA             string
	BrokerNamespace      string
	BrokerKubeconfigPath string
	AllocatedVtepCIDR    string
	AllocatedASN         int32
	PoolASNMin           int32
	PoolASNMax           int32
	PoolVtepCIDR         string
	PoolVtepPrefixLen    int
}

func LoadEnv() FromEnvironment {
	out := FromEnvironment{
		ClusterID:            strings.TrimSpace(os.Getenv(EnvClusterID)),
		BrokerAPIServer:      strings.TrimSpace(os.Getenv(EnvBrokerAPIServer)),
		BrokerToken:          strings.TrimSpace(os.Getenv(EnvBrokerToken)),
		BrokerCA:             os.Getenv(EnvBrokerCA),
		BrokerNamespace:      strings.TrimSpace(os.Getenv(EnvBrokerNamespace)),
		BrokerKubeconfigPath: strings.TrimSpace(os.Getenv(EnvBrokerKubeconfig)),
		AllocatedVtepCIDR:    strings.TrimSpace(os.Getenv(EnvAllocatedVtepCIDR)),
	}
	if out.BrokerNamespace == "" {
		out.BrokerNamespace = DefaultBrokerNamespace
	}
	if s := strings.TrimSpace(os.Getenv(EnvAllocatedASN)); s != "" {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil {
			out.AllocatedASN = int32(v)
		}
	}
	if s := strings.TrimSpace(os.Getenv(EnvPoolASNMin)); s != "" {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil {
			out.PoolASNMin = int32(v)
		}
	}
	if s := strings.TrimSpace(os.Getenv(EnvPoolASNMax)); s != "" {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil {
			out.PoolASNMax = int32(v)
		}
	}
	out.PoolVtepCIDR = strings.TrimSpace(os.Getenv(EnvPoolVtepCIDR))
	if s := strings.TrimSpace(os.Getenv(EnvPoolVtepPrefixLen)); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			out.PoolVtepPrefixLen = v
		}
	}
	return out
}

// HasBrokerTokenConfig reports whether server+token broker access is configured via env.
func (e FromEnvironment) HasBrokerTokenConfig() bool {
	return e.BrokerAPIServer != "" && e.BrokerToken != ""
}

// HasPreallocatedNetwork reports whether VTEP + ASN are set (operator mirrors Skynet status into these env vars).
func (e FromEnvironment) HasPreallocatedNetwork() bool {
	return e.AllocatedVtepCIDR != "" && e.AllocatedASN != 0
}

// HasBrokerPoolEnv reports whether pool mirror env vars are set (must match broker ConfigMap skynet-broker-pools).
func (e FromEnvironment) HasBrokerPoolEnv() bool {
	return e.PoolASNMin != 0 && e.PoolASNMax != 0 && e.PoolVtepCIDR != "" && e.PoolVtepPrefixLen != 0
}
