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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/agent/allocator"
	"github.com/aswinsuryana/skynet/pkg/agent/bgp"
	"github.com/aswinsuryana/skynet/pkg/agent/endpoint"
	"github.com/aswinsuryana/skynet/pkg/agent/syncer"
	"github.com/aswinsuryana/skynet/pkg/agent/vtep"
)

const (
	// HeartbeatInterval is how often to update the Cluster CR heartbeat
	HeartbeatInterval = 30 * time.Second
	// ReconcileInterval is how often to reconcile the cluster state
	ReconcileInterval = 1 * time.Minute
)

// Agent represents the SkyNet agent running in a cluster
type Agent struct {
	clusterID       string
	localClient     dynamic.Interface
	localK8sClient  kubernetes.Interface
	localConfig     *rest.Config
	restMapper      meta.RESTMapper
	brokerClient    dynamic.Interface
	brokerConfig    *rest.Config
	brokerNS        string
	mgr             manager.Manager

	// Components
	brokerSyncer     *syncer.BrokerSyncer
	vtepAllocator    *allocator.VtepAllocator
	asnAllocator     *allocator.ASNAllocator
	vtepManager      *vtep.VtepManager
	bgpConfigurator  *bgp.BGPConfigurator
	endpointReporter *endpoint.EndpointReporter

	// State
	cluster *skynetv1.Cluster
}

// Config holds configuration for the Agent
type Config struct {
	ClusterID      string
	LocalClient    dynamic.Interface
	LocalK8sClient kubernetes.Interface
	LocalConfig    *rest.Config
	RestMapper     meta.RESTMapper
	BrokerClient   dynamic.Interface
	BrokerConfig   *rest.Config
	BrokerNS       string
	Manager        manager.Manager
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

	agent := &Agent{
		clusterID:      config.ClusterID,
		localClient:    config.LocalClient,
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

	// Initialize VTEP allocator
	a.vtepAllocator = allocator.NewVtepAllocator(a.brokerClient, a.brokerNS, a.clusterID)

	// Initialize ASN allocator
	a.asnAllocator = allocator.NewASNAllocator(a.brokerClient, a.brokerNS, a.clusterID)

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

// registerCluster creates or updates the Cluster CR on the broker
func (a *Agent) registerCluster(ctx context.Context) error {
	klog.Infof("Registering cluster %s with broker", a.clusterID)

	// Check if cluster already exists in broker
	clusterUnstructured, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Get(ctx, a.clusterID, metav1.GetOptions{})

	var cluster *skynetv1.Cluster
	if err == nil {
		// Cluster exists, convert it
		cluster = &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterUnstructured.Object, cluster); err != nil {
			return errors.Wrap(err, "failed to convert existing Cluster")
		}
		klog.V(2).Infof("Found existing Cluster CR for %s", a.clusterID)
	} else {
		// Cluster doesn't exist, create new one
		cluster = &skynetv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "skynet.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: a.clusterID,
			},
			Spec: skynetv1.ClusterSpec{
				ClusterID: a.clusterID,
			},
		}
		klog.V(2).Infof("Creating new Cluster CR for %s", a.clusterID)
	}

	// Allocate VTEP CIDR if needed
	if cluster.Spec.VtepCIDR == "" {
		vtepCIDR, err := a.vtepAllocator.AllocateVtepCIDR(ctx, cluster)
		if err != nil {
			return errors.Wrap(err, "failed to allocate VTEP CIDR")
		}
		cluster.Spec.VtepCIDR = vtepCIDR
		klog.Infof("Allocated VTEP CIDR %s for cluster %s", vtepCIDR, a.clusterID)
	}

	// Allocate ASN if needed
	if cluster.Spec.ASN == 0 {
		asn, err := a.asnAllocator.AllocateASN(ctx, cluster)
		if err != nil {
			return errors.Wrap(err, "failed to allocate ASN")
		}
		cluster.Spec.ASN = asn
		klog.Infof("Allocated ASN %d for cluster %s", asn, a.clusterID)
	}

	// Initialize status
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = skynetv1.ClusterPhasePending
	}
	cluster.Status.LastHeartbeat = metav1.NewTime(time.Now())

	// Store the cluster CR
	a.cluster = cluster

	// Create or update on broker (using optimistic locking)
	unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to convert Cluster to unstructured")
	}

	if clusterUnstructured == nil {
		// Create new cluster
		_, err = a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Create(ctx, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to create Cluster on broker")
		}
		klog.Infof("Successfully created Cluster CR on broker for %s", a.clusterID)
	} else {
		// Update existing cluster
		clusterUnstructured.Object = unstructuredCluster
		_, err = a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to update Cluster on broker")
		}
		klog.Infof("Successfully updated Cluster CR on broker for %s", a.clusterID)
	}

	// Also create locally for syncer to pick up
	localUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to convert Cluster to unstructured for local")
	}

	_, err = a.localClient.Resource(skynetv1.ClusterGVR).Namespace(metav1.NamespaceDefault).Create(ctx, &unstructured.Unstructured{Object: localUnstructured}, metav1.CreateOptions{})
	if err != nil {
		// Ignore already exists errors
		klog.V(2).Infof("Local Cluster CR may already exist: %v", err)
	}

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

	// Collect node endpoints
	endpoints, err := a.endpointReporter.CollectEndpoints(ctx, a.vtepManager.AllocateVtepIP)
	if err != nil {
		return errors.Wrap(err, "failed to collect node endpoints")
	}

	// Update Cluster CR status with endpoints
	// Don't fail reconciliation if update fails due to concurrent modification
	// The next reconciliation will retry
	if err := a.updateClusterStatus(ctx, endpoints); err != nil {
		klog.Warningf("Failed to update cluster status (will retry): %v", err)
	}

	// Get remote clusters from broker
	// Remote cluster endpoint info is used directly for BGP configuration
	remoteClusters, err := a.brokerSyncer.GetRemoteClusters(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get remote clusters")
	}

	// Get MultiClusterNetworks from local cache
	// For now, we'll skip this and implement it when we need it
	var multiClusterNetworks []*skynetv1.MultiClusterNetwork

	// Reconcile BGP configuration
	// BGP configurator reads remote cluster endpoints from remoteClusters
	// No need for separate VTEP CRs - we get endpoint info from broker Cluster CRs
	if err := a.bgpConfigurator.ReconcileBGPConfig(ctx, remoteClusters, multiClusterNetworks); err != nil {
		return errors.Wrap(err, "failed to reconcile BGP config")
	}

	klog.V(4).Info("Cluster state reconciled successfully")
	return nil
}

// updateClusterStatus updates the Cluster CR status with node endpoints
// Uses retry logic to handle concurrent updates from heartbeat
func (a *Agent) updateClusterStatus(ctx context.Context, endpoints []skynetv1.NodeEndpoint) error {
	maxRetries := 3
	backoff := time.Second

	for i := 0; i < maxRetries; i++ {
		// Get latest cluster from broker
		clusterUnstructured, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Get(ctx, a.clusterID, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to get Cluster from broker")
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
		_, err = a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
		if err == nil {
			klog.V(4).Infof("Updated Cluster status with %d endpoints", len(endpoints))
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

// updateHeartbeat updates the LastHeartbeat timestamp on the Cluster CR
func (a *Agent) updateHeartbeat(ctx context.Context) error {
	if a.cluster == nil {
		return errors.New("cluster CR not initialized")
	}

	// Get latest cluster from broker
	clusterUnstructured, err := a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Get(ctx, a.clusterID, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get Cluster from broker")
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
	_, err = a.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(a.brokerNS).Update(ctx, clusterUnstructured, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update heartbeat")
	}

	klog.V(4).Infof("Updated heartbeat for cluster %s", a.clusterID)
	return nil
}

// Stop stops the agent
func (a *Agent) Stop() {
	klog.Info("Stopping SkyNet Agent")
	if a.brokerSyncer != nil {
		a.brokerSyncer.Stop()
	}
}

// GetCluster returns the current Cluster CR
func (a *Agent) GetCluster() *skynetv1.Cluster {
	return a.cluster
}
