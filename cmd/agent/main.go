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

package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/agent"
	"github.com/aswinsuryana/skynet/pkg/agent/controller"
)

var (
	clusterID        string
	brokerKubeconfig string
	brokerNamespace  string
	brokerServer     string
	brokerToken      string
	brokerCAData     string
)

func init() {
	flag.StringVar(&clusterID, "cluster-id", "", "Unique identifier for this cluster")
	flag.StringVar(&brokerKubeconfig, "broker-kubeconfig", "", "Path to broker kubeconfig file")
	flag.StringVar(&brokerNamespace, "broker-namespace", "skynet-broker", "Namespace on broker cluster for SkyNet resources")
	flag.StringVar(&brokerServer, "broker-server", "", "Broker API server URL (alternative to kubeconfig)")
	flag.StringVar(&brokerToken, "broker-token", "", "Broker service account token (alternative to kubeconfig)")
	flag.StringVar(&brokerCAData, "broker-ca-data", "", "Broker CA certificate data (alternative to kubeconfig)")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if clusterID == "" {
		klog.Fatal("--cluster-id is required")
	}

	klog.Infof("Starting SkyNet Agent for cluster: %s", clusterID)

	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Get local cluster config
	localConfig := config.GetConfigOrDie()

	// Create local clients
	localDynamicClient, err := dynamic.NewForConfig(localConfig)
	if err != nil {
		klog.Fatalf("Failed to create local dynamic client: %v", err)
	}

	localK8sClient, err := kubernetes.NewForConfig(localConfig)
	if err != nil {
		klog.Fatalf("Failed to create local kubernetes client: %v", err)
	}

	// Get broker cluster config
	brokerConfig, err := getBrokerConfig()
	if err != nil {
		klog.Fatalf("Failed to get broker config: %v", err)
	}

	// Create broker client
	brokerClient, err := dynamic.NewForConfig(brokerConfig)
	if err != nil {
		klog.Fatalf("Failed to create broker client: %v", err)
	}

	// Setup controller manager
	scheme := runtime.NewScheme()
	if err := skynetv1.AddToScheme(scheme); err != nil {
		klog.Fatalf("Failed to add skynet scheme: %v", err)
	}

	mgr, err := ctrl.NewManager(localConfig, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		klog.Fatalf("Failed to create manager: %v", err)
	}

	// Setup MCNC controller
	mcncReconciler := &controller.MCNCReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		DynamicClient: localDynamicClient,
		K8sClient:     localK8sClient,
		ClusterID:     clusterID,
	}
	if err := mcncReconciler.SetupWithManager(mgr); err != nil {
		klog.Fatalf("Failed to setup MCNC controller: %v", err)
	}

	// Create and start agent
	agentConfig := &agent.Config{
		ClusterID:      clusterID,
		LocalClient:    localDynamicClient,
		LocalK8sClient: localK8sClient,
		BrokerClient:   brokerClient,
		BrokerNS:       brokerNamespace,
		Manager:        mgr,
	}

	skynetAgent, err := agent.NewAgent(agentConfig)
	if err != nil {
		klog.Fatalf("Failed to create agent: %v", err)
	}

	if err := skynetAgent.Start(ctx); err != nil {
		klog.Fatalf("Failed to start agent: %v", err)
	}

	// Start manager
	go func() {
		if err := mgr.Start(ctx); err != nil {
			klog.Fatalf("Failed to start manager: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()

	klog.Info("Shutting down SkyNet Agent")
	skynetAgent.Stop()
}

// getBrokerConfig returns the REST config for the broker cluster
func getBrokerConfig() (*rest.Config, error) {
	// Try kubeconfig file first
	if brokerKubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", brokerKubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load broker kubeconfig: %w", err)
		}
		klog.Info("Using broker kubeconfig file")
		return config, nil
	}

	// Try server/token/ca-data
	if brokerServer != "" && brokerToken != "" {
		config := &rest.Config{
			Host:        brokerServer,
			BearerToken: brokerToken,
		}

		if brokerCAData != "" {
			config.TLSClientConfig = rest.TLSClientConfig{
				CAData: []byte(brokerCAData),
			}
		} else {
			config.TLSClientConfig = rest.TLSClientConfig{
				Insecure: true,
			}
			klog.Warning("No broker CA data provided, using insecure TLS")
		}

		klog.Info("Using broker server/token configuration")
		return config, nil
	}

	return nil, fmt.Errorf("broker configuration not provided - need either --broker-kubeconfig or --broker-server/--broker-token")
}
