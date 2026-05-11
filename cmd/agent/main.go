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

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/submariner-io/admiral/pkg/util"
	skynetv1 "github.com/yboaron/mcn/pkg/apis/skynet.io/v1"
	"github.com/yboaron/mcn/pkg/agent"
	"github.com/yboaron/mcn/pkg/agent/allocator"
	"github.com/yboaron/mcn/pkg/agent/bootstrap"
	"github.com/yboaron/mcn/pkg/agent/controller"
	"github.com/yboaron/mcn/pkg/agent/cudn"
	"github.com/yboaron/mcn/pkg/agent/mcn"
	"github.com/yboaron/mcn/pkg/agent/routeadv"
	"github.com/yboaron/mcn/pkg/operator/alloc"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	env := bootstrap.LoadEnv()
	if env.ClusterID == "" {
		klog.Fatal("cluster id is required (set env " + bootstrap.EnvClusterID + ")")
	}
	clusterID := env.ClusterID
	brokerNamespace := env.BrokerNamespace
	if !env.HasPreallocatedNetwork() {
		klog.Fatalf("Operator must inject VTEP and ASN: set env %s and %s (from Skynet status)",
			bootstrap.EnvAllocatedVtepCIDR, bootstrap.EnvAllocatedASN)
	}
	if !env.HasBrokerPoolEnv() {
		klog.Fatalf("Operator must inject broker pool bounds: set env %s, %s, %s, %s (from broker ConfigMap %s)",
			bootstrap.EnvPoolASNMin, bootstrap.EnvPoolASNMax, bootstrap.EnvPoolVtepCIDR, bootstrap.EnvPoolVtepPrefixLen,
			alloc.BrokerPoolsConfigMap)
	}

	pools, err := alloc.PoolsFromValues(env.PoolASNMin, env.PoolASNMax, env.PoolVtepCIDR, env.PoolVtepPrefixLen)
	if err != nil {
		klog.Fatalf("Invalid broker pool env: %v", err)
	}

	klog.Infof("Starting SkyNet Agent for cluster: %s", clusterID)

	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Get local cluster config
	localConfig := config.GetConfigOrDie()

	// Build REST mapper for type discovery (required by Admiral broker syncer)
	restMapper, err := util.BuildRestMapper(localConfig)
	if err != nil {
		klog.Fatalf("Failed to build REST mapper: %v", err)
	}

	// Create local clients
	localDynamicClient, err := dynamic.NewForConfig(localConfig)
	if err != nil {
		klog.Fatalf("Failed to create local dynamic client: %v", err)
	}

	localK8sClient, err := kubernetes.NewForConfig(localConfig)
	if err != nil {
		klog.Fatalf("Failed to create local kubernetes client: %v", err)
	}

	brokerConfig, err := getBrokerConfig(&env)
	if err != nil {
		klog.Fatalf("Failed to get broker config: %v", err)
	}

	// Create broker client
	brokerClient, err := dynamic.NewForConfig(brokerConfig)
	if err != nil {
		klog.Fatalf("Failed to create broker client: %v", err)
	}

	// Use global Kubernetes scheme and add our CRD types (Submariner pattern)
	// The global scheme already has core types registered
	if err := skynetv1.AddToScheme(clientgoscheme.Scheme); err != nil {
		klog.Fatalf("Failed to add SkyNet types to scheme: %v", err)
	}

	// Setup controller manager with the global scheme
	mgr, err := ctrl.NewManager(localConfig, ctrl.Options{
		Scheme: clientgoscheme.Scheme,
	})
	if err != nil {
		klog.Fatalf("Failed to create manager: %v", err)
	}

	// Initialize CUDN stretching components
	klog.Info("Initializing CUDN stretching components")

	// VNI Allocator - allocates VNI from range 5000-10000 on broker
	vniAllocator := allocator.NewVNIAllocator(brokerClient, brokerNamespace, clusterID)

	// MCN Manager - handles MultiClusterNetwork create/join/leave with finalizers
	mcnManager, err := mcn.NewMCNManager(&mcn.Config{
		BrokerClient: brokerClient,
		BrokerNS:     brokerNamespace,
		ClusterID:    clusterID,
		VNIAllocator: vniAllocator,
	})
	if err != nil {
		klog.Fatalf("Failed to create MCN manager: %v", err)
	}

	// CUDN Integrator - injects EVPN config into existing CUDNs
	vtepName := "skynet-local" // Name of local VTEP CR created by agent
	cudnIntegrator, err := cudn.NewCUDNIntegrator(&cudn.Config{
		DynamicClient: localDynamicClient,
		VTEPName:      vtepName,
	})
	if err != nil {
		klog.Fatalf("Failed to create CUDN integrator: %v", err)
	}

	// RouteAdvertisement Creator - creates RouteAdvertisement CRs to trigger OVN-K FRRConfiguration generation
	routeAdvCreator, err := routeadv.NewCreator(&routeadv.Config{
		DynamicClient: localDynamicClient,
		ClusterName:   clusterID,
	})
	if err != nil {
		klog.Fatalf("Failed to create RouteAdvertisement creator: %v", err)
	}

	klog.Info("CUDN stretching components initialized successfully")

	// Setup MCNC controller with all components
	mcncReconciler := &controller.MCNCReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		DynamicClient:    localDynamicClient,
		K8sClient:        localK8sClient,
		ClusterID:        clusterID,
		BrokerClient:     brokerClient,
		BrokerNamespace:  brokerNamespace,
		VTEPName:         vtepName,
		VNIAllocator:     vniAllocator,
		MCNManager:       mcnManager,
		CUDNIntegrator:   cudnIntegrator,
		RouteAdvCreator:  routeAdvCreator,
	}
	if err := mcncReconciler.SetupWithManager(mgr); err != nil {
		klog.Fatalf("Failed to setup MCNC controller: %v", err)
	}

	klog.Info("MCNC controller configured with CUDN stretching support")

	klog.Infof("Using operator allocation from env (%s, %s)",
		bootstrap.EnvAllocatedVtepCIDR, bootstrap.EnvAllocatedASN)

	agentConfig := &agent.Config{
		ClusterID:        clusterID,
		OperatorVtepCIDR: env.AllocatedVtepCIDR,
		OperatorASN:      env.AllocatedASN,
		BrokerPools:      pools,
		LocalClient:          localDynamicClient,
		LocalK8sClient:       localK8sClient,
		LocalConfig:          localConfig,
		RestMapper:           restMapper,
		BrokerClient:         brokerClient,
		BrokerConfig:         brokerConfig,
		BrokerNS:             brokerNamespace,
		Manager:              mgr,
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

// getBrokerConfig returns the REST config for the broker cluster from environment.
// Either SKYNET_BROKER_KUBECONFIG (path) or SKYNET_BROKER_API_SERVER + SKYNET_BROKER_TOKEN (Submariner-style).
func getBrokerConfig(env *bootstrap.FromEnvironment) (*rest.Config, error) {
	if env.BrokerKubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", env.BrokerKubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load broker kubeconfig: %w", err)
		}
		klog.Info("Using broker kubeconfig from " + bootstrap.EnvBrokerKubeconfig)
		return config, nil
	}

	if env.BrokerAPIServer != "" && env.BrokerToken != "" {
		config := &rest.Config{
			Host:        env.BrokerAPIServer,
			BearerToken: env.BrokerToken,
		}
		if env.BrokerCA != "" {
			config.TLSClientConfig = rest.TLSClientConfig{
				CAData: []byte(env.BrokerCA),
			}
		} else {
			config.TLSClientConfig = rest.TLSClientConfig{
				Insecure: true,
			}
			klog.Warning("No broker CA data provided, using insecure TLS")
		}
		klog.Info("Using broker API server and token from environment")
		return config, nil
	}

	return nil, fmt.Errorf("broker configuration not provided (set %s and %s, or %s)",
		bootstrap.EnvBrokerAPIServer, bootstrap.EnvBrokerToken, bootstrap.EnvBrokerKubeconfig)
}
