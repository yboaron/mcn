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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

// Skynet is the primary install/configuration resource for SkyNet on a cluster, analogous to
// submariner.io/v1 Submariner. The skynet-operator watches this CR, reconciles broker connectivity,
// allocates VTEP CIDR and ASN (or validates spec overrides), writes them to status, and creates the
// skynet-agent Deployment with configuration via environment variables (see docs/SKYNET_OPERATOR_CONTRACT.md).
//
// The skynet-agent requires SKYNET_ALLOCATED_VTEP_CIDR and SKYNET_ALLOCATED_ASN (mirrored from status);
// broker/cluster identity may still use flags or env.
type Skynet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SkynetSpec   `json:"spec"`
	Status            SkynetStatus `json:"status,omitempty"`
}

type SkynetSpec struct {
	// ClusterID is the unique identifier for this cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	ClusterID string `json:"clusterID"`

	// BrokerConfig contains the broker connection details
	// +kubebuilder:validation:Required
	BrokerConfig BrokerConfig `json:"brokerConfig"`

	// VtepCIDR is an optional user-supplied VTEP CIDR for this cluster (/16 within the deployment VTEP pool).
	// When empty, the operator allocates one from the broker pool.
	// +optional
	VtepCIDR string `json:"vtepCIDR,omitempty"`

	// ASN is an optional user-supplied BGP private ASN (RFC 6996 range). When zero, the operator allocates one.
	// +optional
	// +kubebuilder:validation:Minimum=64512
	// +kubebuilder:validation:Maximum=65534
	ASN int32 `json:"asn,omitempty"`

	// RouteReflectorCount is the number of Route Reflector nodes
	// 0 = full mesh mode, >0 = RR mode with auto-selection
	// +optional
	// +kubebuilder:default=0
	RouteReflectorCount int `json:"routeReflectorCount,omitempty"`

	// Agent controls how the operator deploys skynet-agent on this cluster (image, replicas).
	// +optional
	Agent *SkynetAgentSpec `json:"agent,omitempty"`
}

// SkynetAgentSpec is input for the operator when creating the skynet-agent Deployment.
type SkynetAgentSpec struct {
	// Image overrides the agent container image. Empty means the operator default (e.g. RELATED_IMAGE_AGENT).
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas is the agent Deployment replicas (typically 1).
	// +optional
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`
}

type BrokerConfig struct {
	// Server is the broker API server URL
	// +kubebuilder:validation:Required
	Server string `json:"server"`

	// BrokerNamespace is the namespace on the broker cluster where SkyNet stores Cluster and related CRs.
	// Defaults to "skynet-broker" when empty (same default as SKYNET_BROKER_NAMESPACE on the agent).
	// +optional
	BrokerNamespace string `json:"brokerNamespace,omitempty"`

	// TokenSecretRef is the reference to the secret containing broker credentials
	// +kubebuilder:validation:Required
	TokenSecretRef SecretReference `json:"tokenSecretRef"`
}

type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

type SkynetStatus struct {
	// Phase represents the current state of the SkyNet configuration
	// +optional
	Phase SkynetPhase `json:"phase,omitempty"`

	// VtepCIDR is the effective VTEP CIDR after operator allocation (or spec override).
	// The operator should mirror this into the agent pod env SKYNET_ALLOCATED_VTEP_CIDR.
	// +optional
	VtepCIDR string `json:"vtepCIDR,omitempty"`

	// ASN is the effective BGP ASN after operator allocation (or spec override).
	// The operator should mirror this into the agent pod env SKYNET_ALLOCATED_ASN.
	// +optional
	ASN int32 `json:"asn,omitempty"`

	// AgentReady indicates the operator considers the skynet-agent Deployment available.
	// +optional
	AgentReady bool `json:"agentReady,omitempty"`

	// ObservedGeneration is the metadata.generation last fully reconciled by the operator.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the Skynet state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type SkynetPhase string

const (
	SkynetPhasePending SkynetPhase = "Pending"
	SkynetPhaseActive  SkynetPhase = "Active"
	SkynetPhaseError   SkynetPhase = "Error"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkynetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skynet `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

// Cluster represents cluster registration and endpoints in the broker.
// Created and managed by SkyNet agent.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

type ClusterSpec struct {
	// ClusterID is the unique identifier for this cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	ClusterID string `json:"clusterID"`

	// VtepCIDR is the VTEP CIDR for this cluster on the broker.
	// Set by the skynet-operator (Skynet status → agent env); the agent registers this value on the broker Cluster CR.
	// +kubebuilder:validation:Required
	VtepCIDR string `json:"vtepCIDR"`

	// ASN is the BGP ASN for this cluster on the broker.
	// Set by the skynet-operator (Skynet status → agent env); the agent registers this value on the broker Cluster CR.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=64512
	// +kubebuilder:validation:Maximum=65534
	ASN int32 `json:"asn"`
}

type ClusterStatus struct {
	// Phase represents the current state of the cluster
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`

	// Endpoints is the list of node endpoints in this cluster
	// +optional
	Endpoints []NodeEndpoint `json:"endpoints,omitempty"`

	// LastHeartbeat is the timestamp of the last update from the cluster
	// +optional
	LastHeartbeat metav1.Time `json:"lastHeartbeat,omitempty"`
}

type ClusterPhase string

const (
	ClusterPhasePending  ClusterPhase = "Pending"
	ClusterPhaseReady    ClusterPhase = "Ready"
	ClusterPhaseDegraded ClusterPhase = "Degraded"
)

type NodeEndpoint struct {
	// Node is the node name
	// +kubebuilder:validation:Required
	Node string `json:"node"`

	// BgpPeerIP is the IP address used for BGP peering
	// +kubebuilder:validation:Required
	BgpPeerIP string `json:"bgpPeerIP"`

	// VtepIPs are the VTEP IP addresses for this node (supports dual-stack)
	// Maximum 2 IPs (one IPv4, one IPv6)
	// TODO(Phase 2): Populate from VTEP CR status when OVN-K VTEP controller is available
	// See: https://github.com/ovn-kubernetes/ovn-kubernetes/pull/6078
	// +optional
	// +kubebuilder:validation:MaxItems=2
	VtepIPs []string `json:"vtepIPs,omitempty"`

	// RouteReflector indicates if this node is a Route Reflector
	// +optional
	RouteReflector bool `json:"routeReflector,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

// +genclient
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

// MultiClusterNetwork represents a multi-cluster network across clusters.
// Created and managed on the broker.
type MultiClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MultiClusterNetworkSpec `json:"spec"`
}

type MultiClusterNetworkSpec struct {
	// VNI is the allocated VXLAN Network Identifier
	// Allocated by first cluster using optimistic locking
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=5000
	// +kubebuilder:validation:Maximum=16777215
	VNI uint32 `json:"vni"`

	// RouteTarget is the BGP EVPN route target in format "<ASN>:<VNI>"
	// Uses fixed ASN 65000 for route targets
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+:\d+$`
	RouteTarget string `json:"routeTarget"`

	// Topology is the network topology type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Layer2;Layer3
	Topology NetworkTopology `json:"topology"`
}

type NetworkTopology string

const (
	NetworkTopologyLayer2 NetworkTopology = "Layer2"
	NetworkTopologyLayer3 NetworkTopology = "Layer3"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MultiClusterNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterNetwork `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

// MultiClusterNetworkConnect is applied locally to connect a CUDN to a multi-cluster network.
// SkyNet agent watches this CR and handles the connection.
type MultiClusterNetworkConnect struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MultiClusterNetworkConnectSpec   `json:"spec"`
	Status            MultiClusterNetworkConnectStatus `json:"status,omitempty"`
}

type MultiClusterNetworkConnectSpec struct {
	// LocalCUDN is the name of the local CUDN to connect
	// +kubebuilder:validation:Required
	LocalCUDN string `json:"localCUDN"`

	// MultiClusterNetworkName is the name of the MultiClusterNetwork to join
	// Mutually exclusive with CreateMultiClusterNetwork
	// +optional
	MultiClusterNetworkName string `json:"multiClusterNetworkName,omitempty"`

	// CreateMultiClusterNetwork specifies parameters for creating a new multi-cluster network
	// Mutually exclusive with MultiClusterNetworkName
	// +optional
	CreateMultiClusterNetwork *CreateMultiClusterNetworkParams `json:"createMultiClusterNetwork,omitempty"`
}

type CreateMultiClusterNetworkParams struct {
	// Name is the name of the MultiClusterNetwork to create
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Topology is the network topology type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Layer2;Layer3
	Topology NetworkTopology `json:"topology"`
}

type MultiClusterNetworkConnectStatus struct {
	// Phase represents the current state of the connection
	// +optional
	Phase MultiClusterNetworkConnectPhase `json:"phase,omitempty"`

	// VNI is the allocated VNI for this multi-cluster network
	// +optional
	VNI uint32 `json:"vni,omitempty"`

	// RouteTarget is the route target for this multi-cluster network
	// +optional
	RouteTarget string `json:"routeTarget,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type MultiClusterNetworkConnectPhase string

const (
	MultiClusterNetworkConnectPhasePending   MultiClusterNetworkConnectPhase = "Pending"
	MultiClusterNetworkConnectPhaseConnected MultiClusterNetworkConnectPhase = "Connected"
	MultiClusterNetworkConnectPhaseError     MultiClusterNetworkConnectPhase = "Error"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MultiClusterNetworkConnectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterNetworkConnect `json:"items"`
}
