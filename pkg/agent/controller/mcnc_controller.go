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

package controller

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

var (
	// UserDefinedNetworkGVR is the GroupVersionResource for UserDefinedNetwork
	UserDefinedNetworkGVR = schema.GroupVersionResource{
		Group:    "k8s.ovn.org",
		Version:  "v1",
		Resource: "userdefinednetworks",
	}
)

// MCNCReconciler reconciles MultiClusterNetworkConnect resources
type MCNCReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	DynamicClient dynamic.Interface
	K8sClient     kubernetes.Interface
	ClusterID     string
}

// Reconcile handles MultiClusterNetworkConnect events
func (r *MCNCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(2).Infof("Reconciling MultiClusterNetworkConnect %s/%s", req.Namespace, req.Name)

	// Fetch the MultiClusterNetworkConnect
	mcnc := &skynetv1.MultiClusterNetworkConnect{}
	if err := r.Get(ctx, req.NamespacedName, mcnc); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(2).Infof("MultiClusterNetworkConnect %s/%s not found, may have been deleted", req.Namespace, req.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to get MultiClusterNetworkConnect")
	}

	// Check if the MCNC is being deleted
	if !mcnc.DeletionTimestamp.IsZero() {
		return r.handleDelete(ctx, mcnc)
	}

	// Reconcile the MCNC
	return r.reconcileMCNC(ctx, mcnc)
}

// reconcileMCNC reconciles a MultiClusterNetworkConnect
func (r *MCNCReconciler) reconcileMCNC(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect) (ctrl.Result, error) {
	klog.V(2).Infof("Reconciling MCNC %s/%s for MCN %s", mcnc.Namespace, mcnc.Name, mcnc.Spec.MultiClusterNetworkName)

	// Fetch the MultiClusterNetwork
	mcn := &skynetv1.MultiClusterNetwork{}
	if err := r.Get(ctx, client.ObjectKey{Name: mcnc.Spec.MultiClusterNetworkName}, mcn); err != nil {
		// Update status to reflect error
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get MultiClusterNetwork %s", mcnc.Spec.MultiClusterNetworkName)
	}

	// Verify namespace exists
	ns, err := r.K8sClient.CoreV1().Namespaces().Get(ctx, mcnc.Namespace, metav1.GetOptions{})
	if err != nil {
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get namespace %s", mcnc.Namespace)
	}

	// Create or update UserDefinedNetwork for this namespace
	if err := r.reconcileUserDefinedNetwork(ctx, mcnc, mcn, ns); err != nil {
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile UserDefinedNetwork")
	}

	// Update status to connected
	mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseConnected
	if err := r.Status().Update(ctx, mcnc); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to update MCNC status")
	}

	klog.Infof("Successfully reconciled MCNC %s/%s", mcnc.Namespace, mcnc.Name)
	return reconcile.Result{}, nil
}

// reconcileUserDefinedNetwork creates or updates the UserDefinedNetwork for a namespace
func (r *MCNCReconciler) reconcileUserDefinedNetwork(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect,
	mcn *skynetv1.MultiClusterNetwork, ns *corev1.Namespace) error {

	udnName := fmt.Sprintf("skynet-%s", mcn.Name)

	// Build UDN spec based on MCN topology
	spec := r.buildUDNSpec(mcn, ns.Name)

	udn := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "UserDefinedNetwork",
			"metadata": map[string]interface{}{
				"name":      udnName,
				"namespace": ns.Name,
				"labels": map[string]interface{}{
					"skynet.io/mcn":        mcnc.Spec.MultiClusterNetworkName,
					"skynet.io/managed-by": "skynet-agent",
				},
			},
			"spec": spec,
		},
	}

	// Check if UDN already exists
	existingUDN, err := r.DynamicClient.Resource(UserDefinedNetworkGVR).Namespace(ns.Name).Get(ctx, udnName, metav1.GetOptions{})
	if err == nil {
		// Update existing UDN
		udn.SetResourceVersion(existingUDN.GetResourceVersion())
		_, err = r.DynamicClient.Resource(UserDefinedNetworkGVR).Namespace(ns.Name).Update(ctx, udn, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update UserDefinedNetwork %s", udnName)
		}
		klog.V(4).Infof("Updated UserDefinedNetwork %s in namespace %s", udnName, ns.Name)
	} else {
		// Create new UDN
		_, err = r.DynamicClient.Resource(UserDefinedNetworkGVR).Namespace(ns.Name).Create(ctx, udn, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create UserDefinedNetwork %s", udnName)
		}
		klog.Infof("Created UserDefinedNetwork %s in namespace %s", udnName, ns.Name)
	}

	return nil
}

// buildUDNSpec builds the UserDefinedNetwork spec based on MCN topology
func (r *MCNCReconciler) buildUDNSpec(mcn *skynetv1.MultiClusterNetwork, namespace string) map[string]interface{} {
	spec := map[string]interface{}{
		"topology": string(mcn.Spec.Topology),
		"layer3": map[string]interface{}{
			"role": "Primary",
			"subnets": []string{
				"10.128.0.0/14",
			},
		},
	}

	if mcn.Spec.Topology == skynetv1.NetworkTopologyLayer2 {
		// For Layer2, configure as a flat L2 network
		spec["layer2"] = map[string]interface{}{
			"role": "Primary",
			"subnets": []string{
				"10.100.0.0/16",
			},
		}
	}

	return spec
}

// handleDelete handles deletion of MultiClusterNetworkConnect
func (r *MCNCReconciler) handleDelete(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect) (ctrl.Result, error) {
	klog.Infof("Handling deletion of MCNC %s/%s", mcnc.Namespace, mcnc.Name)

	// For deletion, we need to find the MCN name from the spec
	mcnName := mcnc.Spec.MultiClusterNetworkName
	if mcnName == "" {
		// If name not set, we can't delete the UDN
		return reconcile.Result{}, nil
	}

	// Delete the UserDefinedNetwork
	udnName := fmt.Sprintf("skynet-%s", mcnName)
	err := r.DynamicClient.Resource(UserDefinedNetworkGVR).Namespace(mcnc.Namespace).Delete(ctx, udnName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, errors.Wrapf(err, "failed to delete UserDefinedNetwork %s", udnName)
	}

	klog.Infof("Deleted UserDefinedNetwork %s from namespace %s", udnName, mcnc.Namespace)
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *MCNCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&skynetv1.MultiClusterNetworkConnect{}).
		Complete(r)
}
