/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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
	"os"
	"os/signal"
	"syscall"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/submariner-io/admiral/pkg/log/kzerolog"
	"github.com/aswinsuryana/skynet/pkg/names"
)

var log = logf.Log.WithName("mcn-operator")

func main() {
	kzerolog.AddFlags(nil)
	flag.Parse()
	kzerolog.InitK8sLogging()

	log.Info("MCN operator starting")

	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "Failed to get in-cluster config")
		os.Exit(1)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := ensureNamespace(ctx, client); err != nil {
		log.Error(err, "Failed to ensure namespace")
		os.Exit(1)
	}

	if err := ensureServiceAccount(ctx, client, names.BrokerServiceAccount); err != nil {
		log.Error(err, "Failed to ensure broker ServiceAccount")
		os.Exit(1)
	}

	if err := ensureServiceAccount(ctx, client, names.AgentServiceAccount); err != nil {
		log.Error(err, "Failed to ensure agent ServiceAccount")
		os.Exit(1)
	}

	if err := ensureDeployment(ctx, client, names.BrokerComponent, brokerImage()); err != nil {
		log.Error(err, "Failed to ensure broker Deployment")
		os.Exit(1)
	}

	if err := ensureDeployment(ctx, client, names.AgentComponent, agentImage()); err != nil {
		log.Error(err, "Failed to ensure agent Deployment")
		os.Exit(1)
	}

	log.Info("MCN operator running — broker and agent deployments are up")

	// Re-check every 30s so manually deleted resources are re-created automatically.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("MCN operator shutting down")
			return
		case <-ticker.C:
			if err := ensureDeployment(ctx, client, names.BrokerComponent, brokerImage()); err != nil {
				log.Error(err, "Failed to reconcile broker Deployment")
			}

			if err := ensureDeployment(ctx, client, names.AgentComponent, agentImage()); err != nil {
				log.Error(err, "Failed to reconcile agent Deployment")
			}
		}
	}
}

func ensureNamespace(ctx context.Context, client kubernetes.Interface) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.Namespace,
			Labels: map[string]string{names.ManagedByLabel: names.ManagedByValue},
		},
	}

	_, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}

		return fmt.Errorf("creating namespace %s: %w", names.Namespace, err)
	}

	log.Info("Created", "Namespace", names.Namespace)

	return nil
}

func ensureServiceAccount(ctx context.Context, client kubernetes.Interface, saName string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: names.Namespace,
			Labels:    componentLabels(saName),
		},
	}

	_, err := client.CoreV1().ServiceAccounts(names.Namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}

		return fmt.Errorf("creating ServiceAccount %s: %w", saName, err)
	}

	log.Info("Created", "ServiceAccount", saName)

	return nil
}

func ensureDeployment(ctx context.Context, client kubernetes.Interface, component, image string) error {
	replicas := int32(1)

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      component,
			Namespace: names.Namespace,
			Labels:    componentLabels(component),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: componentLabels(component),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: componentLabels(component),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: component,
					Containers: []corev1.Container{
						{
							Name:  component,
							Image: image,
						},
					},
				},
			},
		},
	}

	_, err := client.AppsV1().Deployments(names.Namespace).Create(ctx, desired, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}

		return fmt.Errorf("creating Deployment %s: %w", component, err)
	}

	log.Info("Created", "Deployment", component)

	return nil
}

func componentLabels(component string) map[string]string {
	return map[string]string{
		names.AppLabel:       names.AppValue,
		names.ComponentLabel: component,
		names.ManagedByLabel: names.ManagedByValue,
	}
}

func brokerImage() string {
	if img := os.Getenv(names.BrokerImageEnvVar); img != "" {
		return img
	}

	return names.DefaultBrokerImage
}

func agentImage() string {
	if img := os.Getenv(names.AgentImageEnvVar); img != "" {
		return img
	}

	return names.DefaultAgentImage
}
