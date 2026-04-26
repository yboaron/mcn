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

package agent

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/agent/bootstrap"
	"github.com/aswinsuryana/skynet/pkg/agent/bgp"
	"github.com/aswinsuryana/skynet/pkg/agent/endpoint"
	"github.com/aswinsuryana/skynet/pkg/agent/syncer"
	"github.com/aswinsuryana/skynet/pkg/agent/vtep"
	"github.com/aswinsuryana/skynet/pkg/operator/alloc"
)

const (
	// HeartbeatInterval is how often to update the Cluster CR heartbeat
	HeartbeatInterval = 30 * time.Second
	// ReconcileInterval is how often to reconcile the cluster state
	ReconcileInterval = 1 * time.Minute
)

// Agent represents the SkyNet agent running in a cluster
type Agent struct {
	clusterID        string
	operatorVtepCIDR string
	operatorASN      int32
	brokerPools      *alloc.Pools
	localClient          dynamic.Interface
	localK8sClient   kubernetes.Interface
	localConfig      *rest.Config
	restMapper       meta.RESTMapper
	brokerClient     dynamic.Interface
	brokerConfig     *rest.Config
	brokerNS         string
	mgr              manager.Manager

	// Components
	brokerSyncer     *syncer.BrokerSyncer
	vtepManager      *vtep.VtepManager
	bgpConfigurator  *bgp.BGPConfigurator
	endpointReporter *endpoint.EndpointReporter

	// State
	cluster *skynetv1.Cluster
}

// Config holds configuration for the Agent
type Config struct {
	ClusterID string
	// OperatorVtepCIDR and OperatorASN come from env (SKYNET_ALLOCATED_*), written by the operator from Skynet status.
	OperatorVtepCIDR string
	OperatorASN      int32
	// BrokerPools mirrors broker ConfigMap bounds (SKYNET_POOL_* env) for validating allocation.
	BrokerPools *alloc.Pools
	LocalClient          dynamic.Interface
	LocalK8sClient  kubernetes.Interface
	LocalConfig     *rest.Config
	RestMapper      meta.RESTMapper
	BrokerClient    dynamic.Interface
	BrokerConfig    *rest.Config
	BrokerNS        string
	Manager         manager.Manager
}

// NewAgent creates a new SkyNet Agent instance
func NewAgent(config *Config) (*Agent, error) {
	if config.ClusterID == "" {
		return nil, errors.New("clusterID is required")
	}
	if config.LocalClient == nil {
		return nil, errors.New("localClient is required")
	}
	if config.BrokerClient == nil {
		return nil, errors.New("brokerClient is required")
	}
	if config.BrokerNS == "" {
		return nil, errors.New("brokerNS is required")
	}
	if config.BrokerPools == nil {
		return nil, errors.New("brokerPools is required (SKYNET_POOL_* env from operator)")
	}

	agent := &Agent{
		clusterID:        config.ClusterID,
		operatorVtepCIDR: config.OperatorVtepCIDR,
		operatorASN:      config.OperatorASN,
		brokerPools:      config.BrokerPools,
		localClient:          config.LocalClient,
		localK8sClient: config.LocalK8sClient,
		localConfig:    config.LocalConfig,
		restMapper:     config.RestMapper,
		brokerClient:   config.BrokerClient,
		brokerConfig:   config.BrokerConfig,
		brokerNS:       config.BrokerNS,
		mgr:            config.Manager,
	}

	// Initialize components
	if err := agent.initComponents(); err != nil {
		return nil, errors.Wrap(err, "failed to initialize agent components")
	}

	return agent, nil
}

// initComponents initializes all agent components
func (a *Agent) initComponents() error {
	klog.Info("Initializing agent components")

	// Initialize broker syncer (following Submariner Lighthouse pattern)
	brokerSyncerConfig := &syncer.Config{
		ClusterID:    a.clusterID,
		LocalClient:  a.localClient,
		LocalConfig:  a.localConfig,
		RestMapper:   a.restMapper,
		BrokerClient: a.brokerClient,
		BrokerConfig: a.brokerConfig,
		BrokerNS:     a.brokerNS,
		Scheme:       a.mgr.GetScheme(),
	}
	var err error
	a.brokerSyncer, err = syncer.NewBrokerSyncer(brokerSyncerConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create broker syncer")
	}

	klog.Info("Agent components initialized successfully")
	return nil
}

// initRuntimeComponents initializes components that need cluster info (called after registration)
func (a *Agent) initRuntimeComponents() error {
	klog.Info("Initializing runtime components")

	// Initialize VTEP manager
	vtepMgrConfig := &vtep.Config{
		LocalClient: a.localClient,
		ClusterID:   a.clusterID,
		VtepCIDR:    a.cluster.Spec.VtepCIDR,
	}
	var err error
	a.vtepManager, err = vtep.NewVtepManager(vtepMgrConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create VTEP manager")
	}

	// Initialize BGP configurator
	bgpConfig := &bgp.Config{
		LocalClient:  a.localClient,
		BrokerClient: a.brokerClient,
		BrokerNS:     a.brokerNS,
		ClusterID:    a.clusterID,
		LocalASN:     a.cluster.Spec.ASN,
		VtepCIDR:     a.cluster.Spec.VtepCIDR,
		Topology:     bgp.TopologyFullMesh, // Default to full mesh
	}
	a.bgpConfigurator, err = bgp.NewBGPConfigurator(bgpConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create BGP configurator")
	}

	// Initialize endpoint reporter
	endpointConfig := &endpoint.Config{
		K8sClient:              a.localK8sClient,
		ClusterID:              a.clusterID,
		RouteReflectorSelector: "skynet.io/route-reflector=true",
	}
	a.endpointReporter, err = endpoint.NewEndpointReporter(endpointConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create endpoint reporter")
	}

	klog.Info("Runtime components initialized successfully")
	return nil
}

// Start starts the SkyNet agent
func (a *Agent) Start(ctx context.Context) error {
	klog.Infof("Starting SkyNet Agent for cluster %s", a.clusterID)

	// Initialize cluster registration
	if err := a.registerCluster(ctx); err != nil {
		return errors.Wrap(err, "failed to register cluster")
	}

	// Initialize runtime components that depend on cluster info
	if err := a.initRuntimeComponents(); err != nil {
		return errors.Wrap(err, "failed to initialize runtime components")
	}

	// Create local VTEP for this cluster
	// This configures OVN-K to allocate VTEP IPs for local nodes
	if err := a.vtepManager.EnsureLocalVTEP(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure local VTEP")
	}

	// Start broker syncer
	if err := a.brokerSyncer.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start broker syncer")
	}

	// Start periodic reconciliation
	go a.reconcileLoop(ctx)

	// Start heartbeat
	go a.heartbeatLoop(ctx)

	klog.Info("SkyNet Agent started successfully")
	return nil
}

// registerCluster creates or updates the local Cluster CR (syncer will export to broker)
func (a *Agent) registerCluster(ctx context.Context) error {
	klog.Infof("Registering cluster %s locally (will sync to broker)", a.clusterID)

	// Check if local Cluster CR exists
	localNS := "skynet-operator"
	clusterUnstructured, getErr := a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Get(ctx, a.clusterID, metav1.GetOptions{})
	localClusterExists := false
	if getErr == nil {
		localClusterExists = true
	} else if !apierrors.IsNotFound(getErr) {
		return errors.Wrap(getErr, "get local Cluster CR")
	}

	var cluster *skynetv1.Cluster
	if localClusterExists {
		cluster = &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterUnstructured.Object, cluster); err != nil {
			return errors.Wrap(err, "failed to convert existing local Cluster")
		}
		klog.V(2).Infof("Found existing local Cluster CR for %s", a.clusterID)
	} else {
		cluster = &skynetv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "skynet.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      a.clusterID,
				Namespace: localNS,
			},
			Spec: skynetv1.ClusterSpec{
				ClusterID: a.clusterID,
			},
		}
		klog.V(2).Infof("Creating new local Cluster CR for %s", a.clusterID)
	}

	if a.operatorVtepCIDR == "" || a.operatorASN == 0 {
		return errors.Errorf("VTEP CIDR and ASN must be set by the operator: set env %s and %s (from Skynet status)",
			bootstrap.EnvAllocatedVtepCIDR, bootstrap.EnvAllocatedASN)
	}
	if err := alloc.ValidateVtepCIDR(a.operatorVtepCIDR, a.brokerPools); err != nil {
		return errors.Wrap(err, "invalid VTEP CIDR from operator env")
	}
	if err := alloc.ValidateASN(a.operatorASN, a.brokerPools); err != nil {
		return errors.Wrap(err, "invalid ASN from operator env")
	}
	cluster.Spec.VtepCIDR = a.operatorVtepCIDR
	cluster.Spec.ASN = a.operatorASN
	klog.Infof("Registered using operator allocation (env): VTEP %s, ASN %d", cluster.Spec.VtepCIDR, cluster.Spec.ASN)

	// Initialize status
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = skynetv1.ClusterPhasePending
	}
	cluster.Status.LastHeartbeat = metav1.NewTime(time.Now())

	// Store the cluster CR
	a.cluster = cluster

	// Create or update locally (broker syncer will export to broker)
	unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to convert Cluster to unstructured")
	}

	if !localClusterExists {
		// Create new local cluster CR
		_, err = a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Create(ctx, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to create local Cluster CR")
		}
		klog.Infof("Successfully created local Cluster CR for %s (will sync to broker)", a.clusterID)
	} else {
		// Update existing local cluster CR
		clusterUnstructured.Object = unstructuredCluster
		_, err = a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to update local Cluster CR")
		}
		klog.Infof("Successfully updated local Cluster CR for %s", a.clusterID)
	}

	// Note: The broker syncer handles bidirectional sync:
	// - Local Cluster CR (skynet-operator) → Broker (skynet-broker)
	// - Remote Cluster CRs: Broker (skynet-broker) → Local (skynet-operator)

	return nil
}

// reconcileLoop periodically reconciles cluster state
func (a *Agent) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Info("Reconcile loop stopped")
			return
		case <-ticker.C:
			if err := a.reconcile(ctx); err != nil {
				klog.Errorf("Reconciliation failed: %v", err)
			}
		}
	}
}

// reconcile performs a reconciliation of cluster state
func (a *Agent) reconcile(ctx context.Context) error {
	klog.V(4).Info("Reconciling cluster state")

	// Collect node endpoints (Phase 1: BGP peer IPs only)
	// Phase 2: Will include VTEP IPs from VTEP CR status when OVN-K controller is available
	endpoints, err := a.endpointReporter.CollectEndpoints(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to collect node endpoints")
	}

	// Update Cluster CR status with endpoints
	// Don't fail reconciliation if update fails due to concurrent modification
	// The next reconciliation will retry
	if err := a.updateClusterStatus(ctx, endpoints); err != nil {
		klog.Warningf("Failed to update cluster status (will retry): %v", err)
	}

	// Refresh local cluster from API server to get latest endpoints in status
	// This ensures BGP configurator has up-to-date endpoint list for intra-cluster peering
	if err := a.refreshLocalCluster(ctx); err != nil {
		return errors.Wrap(err, "failed to refresh local cluster state")
	}

	// Get remote clusters from local synced copies (skynet-operator namespace)
	// The broker syncer imports remote Cluster CRs from broker to local
	remoteClusters, err := a.getRemoteClustersFromLocal(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get remote clusters from local")
	}

	// Get MultiClusterNetworks from local cache
	// For now, we'll skip this and implement it when we need it
	var multiClusterNetworks []*skynetv1.MultiClusterNetwork

	// Reconcile BGP configuration
	// Full mesh topology: each node peers with all other nodes (intra-cluster iBGP + inter-cluster eBGP)
	// - Intra-cluster: N-1 iBGP sessions (same ASN, FRR ignores self-peering)
	// - Inter-cluster: M eBGP sessions per remote cluster (different ASN, ebgpMultiHop)
	if err := a.bgpConfigurator.ReconcileBGPConfig(ctx, a.cluster, remoteClusters, multiClusterNetworks); err != nil {
		return errors.Wrap(err, "failed to reconcile BGP config")
	}

	klog.V(4).Info("Cluster state reconciled successfully")
	return nil
}

// updateClusterStatus updates the local Cluster CR status with node endpoints
// Uses retry logic to handle concurrent updates from heartbeat
// The broker syncer will sync the updated status to the broker
func (a *Agent) updateClusterStatus(ctx context.Context, endpoints []skynetv1.NodeEndpoint) error {
	maxRetries := 3
	backoff := time.Second
	localNS := "skynet-operator"

	for i := 0; i < maxRetries; i++ {
		// Get latest cluster from local
		clusterUnstructured, err := a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Get(ctx, a.clusterID, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to get local Cluster CR")
		}

		cluster := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterUnstructured.Object, cluster); err != nil {
			return errors.Wrap(err, "failed to convert Cluster")
		}

		// Update endpoints
		cluster.Status.Endpoints = endpoints

		// Convert back and update
		unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
		if err != nil {
			return errors.Wrap(err, "failed to convert Cluster to unstructured")
		}

		clusterUnstructured.Object = unstructuredCluster
		_, err = a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
		if err == nil {
			klog.V(4).Infof("Updated local Cluster status with %d endpoints", len(endpoints))
			return nil
		}

		// Check if it's a conflict error
		if !strings.Contains(err.Error(), "object has been modified") {
			return errors.Wrap(err, "failed to update Cluster status")
		}

		// Retry with backoff
		if i < maxRetries-1 {
			klog.V(4).Infof("Cluster status update conflict, retrying in %v (attempt %d/%d)", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	return errors.New("failed to update Cluster status after retries")
}

// heartbeatLoop periodically updates the cluster heartbeat
func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Info("Heartbeat loop stopped")
			return
		case <-ticker.C:
			if err := a.updateHeartbeat(ctx); err != nil {
				klog.Errorf("Failed to update heartbeat: %v", err)
			}
		}
	}
}

// updateHeartbeat updates the LastHeartbeat timestamp on the local Cluster CR
// The broker syncer will sync the updated status to the broker
func (a *Agent) updateHeartbeat(ctx context.Context) error {
	if a.cluster == nil {
		return errors.New("cluster CR not initialized")
	}

	localNS := "skynet-operator"

	// Get latest cluster from local
	clusterUnstructured, err := a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Get(ctx, a.clusterID, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get local Cluster CR")
	}

	cluster := &skynetv1.Cluster{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterUnstructured.Object, cluster); err != nil {
		return errors.Wrap(err, "failed to convert Cluster")
	}

	// Update heartbeat
	cluster.Status.LastHeartbeat = metav1.NewTime(time.Now())
	if cluster.Status.Phase == skynetv1.ClusterPhasePending {
		cluster.Status.Phase = skynetv1.ClusterPhaseReady
	}

	// Convert back and update
	unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to convert Cluster to unstructured")
	}

	clusterUnstructured.Object = unstructuredCluster
	_, err = a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update local heartbeat")
	}

	klog.V(4).Infof("Updated local heartbeat for cluster %s", a.clusterID)
	return nil
}

// Stop stops the agent
func (a *Agent) Stop() {
	klog.Info("Stopping SkyNet Agent")
	if a.brokerSyncer != nil {
		a.brokerSyncer.Stop()
	}
}

// getRemoteClustersFromLocal retrieves remote Cluster CRs from local synced copies
// These are synced from the broker by the broker syncer to skynet-operator namespace
func (a *Agent) getRemoteClustersFromLocal(ctx context.Context) ([]*skynetv1.Cluster, error) {
	localNS := "skynet-operator"

	clusterList, err := a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list local Cluster CRs")
	}

	var remoteClusters []*skynetv1.Cluster
	for i := range clusterList.Items {
		cluster := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterList.Items[i].Object, cluster); err != nil {
			klog.Errorf("Failed to convert Cluster: %v", err)
			continue
		}

		// Skip local cluster (only return remote clusters)
		if cluster.Spec.ClusterID == a.clusterID {
			continue
		}

		remoteClusters = append(remoteClusters, cluster)
	}

	klog.V(4).Infof("Found %d remote clusters in local sync", len(remoteClusters))
	return remoteClusters, nil
}

// refreshLocalCluster refreshes the in-memory cluster state from the API server
// This ensures we have the latest endpoints and status for BGP configuration
func (a *Agent) refreshLocalCluster(ctx context.Context) error {
	localNS := "skynet-operator"

	// Get latest cluster from local API server
	clusterUnstructured, err := a.localClient.Resource(skynetv1.ClusterGVR).Namespace(localNS).Get(ctx, a.clusterID, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get local Cluster CR")
	}

	cluster := &skynetv1.Cluster{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterUnstructured.Object, cluster); err != nil {
		return errors.Wrap(err, "failed to convert Cluster")
	}

	// Update in-memory cluster
	a.cluster = cluster
	klog.V(4).Infof("Refreshed local cluster state: %d endpoints", len(cluster.Status.Endpoints))

	return nil
}

// GetCluster returns the current Cluster CR
func (a *Agent) GetCluster() *skynetv1.Cluster {
	return a.cluster
}
