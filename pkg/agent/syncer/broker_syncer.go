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

package syncer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/admiral/pkg/syncer/broker"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

// BrokerSyncer handles synchronization of resources between local cluster and broker
type BrokerSyncer struct {
	clusterID     string
	localClient   dynamic.Interface
	localConfig   *rest.Config
	restMapper    meta.RESTMapper
	brokerClient  dynamic.Interface
	brokerConfig  *rest.Config
	brokerNS      string
	scheme        *runtime.Scheme
	clusterSyncer *broker.Syncer
	mcnSyncer     *broker.Syncer
}

// Config holds configuration for the BrokerSyncer
type Config struct {
	ClusterID    string
	LocalClient  dynamic.Interface
	LocalConfig  *rest.Config
	RestMapper   meta.RESTMapper
	BrokerClient dynamic.Interface
	BrokerConfig *rest.Config
	BrokerNS     string
	Scheme       *runtime.Scheme
}

// NewBrokerSyncer creates a new BrokerSyncer instance
func NewBrokerSyncer(config *Config) (*BrokerSyncer, error) {
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

	return &BrokerSyncer{
		clusterID:    config.ClusterID,
		localClient:  config.LocalClient,
		localConfig:  config.LocalConfig,
		restMapper:   config.RestMapper,
		brokerClient: config.BrokerClient,
		brokerConfig: config.BrokerConfig,
		brokerNS:     config.BrokerNS,
		scheme:       config.Scheme,
	}, nil
}

// Start initializes and starts the broker syncers
func (s *BrokerSyncer) Start(ctx context.Context) error {
	klog.Infof("Starting BrokerSyncer for cluster %s", s.clusterID)

	// Initialize Cluster syncer
	if err := s.initClusterSyncer(); err != nil {
		return errors.Wrap(err, "failed to initialize Cluster syncer")
	}

	// Initialize MultiClusterNetwork syncer
	if err := s.initMultiClusterNetworkSyncer(); err != nil {
		return errors.Wrap(err, "failed to initialize MultiClusterNetwork syncer")
	}

	// Start the syncers
	if err := s.clusterSyncer.Start(ctx.Done()); err != nil {
		return errors.Wrap(err, "failed to start Cluster syncer")
	}

	if err := s.mcnSyncer.Start(ctx.Done()); err != nil {
		return errors.Wrap(err, "failed to start MultiClusterNetwork syncer")
	}

	klog.Infof("BrokerSyncer started successfully for cluster %s", s.clusterID)
	return nil
}

// initClusterSyncer initializes the Cluster resource syncer
func (s *BrokerSyncer) initClusterSyncer() error {
	klog.V(2).Info("Initializing Cluster syncer")

	// Create broker syncer for Cluster resources
	// Direction: Local -> Broker (push only)
	// Following Submariner Lighthouse pattern
	syncer, err := broker.NewSyncer(broker.SyncerConfig{
		LocalRestConfig:  s.localConfig,
		LocalClient:      s.localClient,
		LocalNamespace:   metav1.NamespaceAll,
		LocalClusterID:   s.clusterID,
		RestMapper:       s.restMapper,
		BrokerRestConfig: s.brokerConfig,
		BrokerClient:     s.brokerClient,
		BrokerNamespace:  s.brokerNS,
		ResourceConfigs: []broker.ResourceConfig{
			{
				LocalSourceNamespace: metav1.NamespaceAll,
				LocalResourceType:    &skynetv1.Cluster{},
				BrokerResourceType:   &skynetv1.Cluster{},
				// Simple passthrough - no transformation needed for Cluster CRs
			},
		},
		Scheme: s.scheme,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create Cluster syncer")
	}

	s.clusterSyncer = syncer
	return nil
}

// initMultiClusterNetworkSyncer initializes the MultiClusterNetwork resource syncer
func (s *BrokerSyncer) initMultiClusterNetworkSyncer() error {
	klog.V(2).Info("Initializing MultiClusterNetwork syncer")

	// Create broker syncer for MultiClusterNetwork resources
	// Direction: Broker -> Local (pull only)
	// Following Submariner Lighthouse pattern
	syncer, err := broker.NewSyncer(broker.SyncerConfig{
		LocalRestConfig:  s.localConfig,
		LocalClient:      s.localClient,
		LocalNamespace:   metav1.NamespaceAll,
		LocalClusterID:   s.clusterID,
		RestMapper:       s.restMapper,
		BrokerRestConfig: s.brokerConfig,
		BrokerClient:     s.brokerClient,
		BrokerNamespace:  s.brokerNS,
		ResourceConfigs: []broker.ResourceConfig{
			{
				LocalSourceNamespace: metav1.NamespaceAll,
				LocalResourceType:    &skynetv1.MultiClusterNetwork{},
				BrokerResourceType:   &skynetv1.MultiClusterNetwork{},
				// No transform needed, just sync as-is from broker to local
			},
		},
		Scheme: s.scheme,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create MultiClusterNetwork syncer")
	}

	s.mcnSyncer = syncer
	return nil
}

// transformCluster is called before syncing Cluster resources to the broker
// It filters to only sync the local cluster's own Cluster CR
func (s *BrokerSyncer) transformCluster(from runtime.Object, _ int, op syncer.Operation) (runtime.Object, bool) {
	cluster, ok := from.(*skynetv1.Cluster)
	if !ok {
		klog.Errorf("Unexpected object type: %T", from)
		return nil, false
	}

	// Only sync our own cluster CR
	if cluster.Spec.ClusterID != s.clusterID {
		klog.V(4).Infof("Skipping Cluster %s - not our cluster", cluster.Spec.ClusterID)
		return nil, false
	}

	klog.V(4).Infof("Syncing Cluster %s to broker", cluster.Spec.ClusterID)
	return cluster, false
}

// GetRemoteClusters retrieves all Cluster resources from the broker (excluding local cluster)
func (s *BrokerSyncer) GetRemoteClusters(ctx context.Context) ([]*skynetv1.Cluster, error) {
	clusterList, err := s.brokerClient.Resource(skynetv1.ClusterGVR).Namespace(s.brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Clusters from broker")
	}

	var remoteClusters []*skynetv1.Cluster
	for i := range clusterList.Items {
		cluster := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterList.Items[i].Object, cluster); err != nil {
			klog.Errorf("Failed to convert Cluster: %v", err)
			continue
		}

		// Skip local cluster
		if cluster.Spec.ClusterID == s.clusterID {
			continue
		}

		remoteClusters = append(remoteClusters, cluster)
	}

	return remoteClusters, nil
}

// Stop stops the broker syncers
func (s *BrokerSyncer) Stop() {
	klog.Info("Stopping BrokerSyncer")
	// Admiral syncers stop automatically when context is done
}
