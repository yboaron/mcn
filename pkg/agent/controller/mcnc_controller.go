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

	skynetv1 "github.com/yboaron/mcn/pkg/apis/skynet.io/v1"
	"github.com/yboaron/mcn/pkg/agent/allocator"
	"github.com/yboaron/mcn/pkg/agent/cudn"
	"github.com/yboaron/mcn/pkg/agent/mcn"
	"github.com/yboaron/mcn/pkg/agent/routeadv"
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
	Scheme           *runtime.Scheme
	DynamicClient    dynamic.Interface
	K8sClient        kubernetes.Interface
	ClusterID        string
	BrokerClient     dynamic.Interface
	BrokerNamespace  string
	VTEPName         string
	VNIAllocator     *allocator.VNIAllocator
	MCNManager       *mcn.MCNManager
	CUDNIntegrator   *cudn.CUDNIntegrator
	RouteAdvCreator  *routeadv.Creator
}

const (
	mcncFinalizerName = "skynet.io/mcnc-cleanup"
)

// Reconcile handles MultiClusterNetworkConnect events
func (r *MCNCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.Infof("Reconciling MultiClusterNetworkConnect %s/%s", req.Namespace, req.Name)

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
		klog.Infof("MCNC %s/%s has deletionTimestamp, triggering cleanup", mcnc.Namespace, mcnc.Name)
		return r.handleDelete(ctx, mcnc)
	}

	// Add finalizer if not present
	if !containsString(mcnc.Finalizers, mcncFinalizerName) {
		mcnc.Finalizers = append(mcnc.Finalizers, mcncFinalizerName)
		if err := r.Update(ctx, mcnc); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer to MCNC")
		}
		klog.V(2).Infof("Added finalizer to MCNC %s/%s", mcnc.Namespace, mcnc.Name)
		return reconcile.Result{Requeue: true}, nil
	}

	// Reconcile the MCNC
	return r.reconcileMCNC(ctx, mcnc)
}

// reconcileMCNC reconciles a MultiClusterNetworkConnect
func (r *MCNCReconciler) reconcileMCNC(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect) (ctrl.Result, error) {
	mcnName := ""
	if mcnc.Spec.CreateMultiClusterNetwork != nil {
		mcnName = mcnc.Spec.CreateMultiClusterNetwork.Name
	}
	klog.V(2).Infof("Reconciling MCNC %s/%s for MCN %s", mcnc.Namespace, mcnc.Name, mcnName)

	// Step 1: Verify namespace exists
	ns, err := r.K8sClient.CoreV1().Namespaces().Get(ctx, mcnc.Namespace, metav1.GetOptions{})
	if err != nil {
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get namespace %s", mcnc.Namespace)
	}

	// Step 2: Get or create MultiClusterNetwork on broker
	mcn, err := r.reconcileMCN(ctx, mcnc)
	if err != nil {
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile MultiClusterNetwork")
	}

	// Step 3: Handle local network (CUDN or UDN)
	cudnName, err := r.reconcileLocalNetwork(ctx, mcnc, mcn, ns)
	if err != nil {
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile local network")
	}

	// Step 4: Create RouteAdvertisement for BGP route advertisement
	if err := r.RouteAdvCreator.CreateForCUDN(ctx, cudnName, mcn); err != nil {
		klog.Errorf("RouteAdvertisement creation failed for CUDN %s: %v", cudnName, err)
		mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseError
		if updateErr := r.Status().Update(ctx, mcnc); updateErr != nil {
			klog.Errorf("Failed to update MCNC status: %v", updateErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to create RouteAdvertisement")
	}

	// Update status to connected
	mcnc.Status.Phase = skynetv1.MultiClusterNetworkConnectPhaseConnected
	if err := r.Status().Update(ctx, mcnc); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to update MCNC status")
	}

	klog.Infof("Successfully reconciled MCNC %s/%s", mcnc.Namespace, mcnc.Name)
	return reconcile.Result{}, nil
}

// reconcileMCN gets or creates the MultiClusterNetwork on the broker
func (r *MCNCReconciler) reconcileMCN(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect) (*skynetv1.MultiClusterNetwork, error) {
	// Determine scenario: join existing or create new
	var mcnName string
	var topology skynetv1.NetworkTopology

	if mcnc.Spec.MultiClusterNetworkName != "" {
		// Join existing MCN
		mcnName = mcnc.Spec.MultiClusterNetworkName
		// Topology will be read from existing MCN
		topology = "" // Will be ignored by CreateOrJoinMCN when MCN exists
	} else if mcnc.Spec.CreateMultiClusterNetwork != nil {
		// Create new MCN (or join if it already exists)
		mcnName = mcnc.Spec.CreateMultiClusterNetwork.Name
		topology = mcnc.Spec.CreateMultiClusterNetwork.Topology
	} else {
		return nil, fmt.Errorf("MCNC must specify either multiClusterNetworkName or createMultiClusterNetwork")
	}

	// Use MCNManager to create or join
	// This handles: VNI allocation, finalizers, race conditions, etc.
	mcn, err := r.MCNManager.CreateOrJoinMCN(ctx, mcnName, topology)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create/join MultiClusterNetwork %s", mcnName)
	}

	klog.Infof("Reconciled MultiClusterNetwork %s (VNI=%d, RT=%s, Topology=%s)",
		mcnName, mcn.Spec.VNI, mcn.Spec.RouteTarget, mcn.Spec.Topology)
	return mcn, nil
}

// reconcileLocalNetwork handles creating CUDN with EVPN or using existing CUDN
// Returns the CUDN/UDN name
func (r *MCNCReconciler) reconcileLocalNetwork(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect,
	mcn *skynetv1.MultiClusterNetwork, ns *corev1.Namespace) (string, error) {

	// Priority 1: LocalCUDN - patch existing CUDN with EVPN (preferred - user owns network definition)
	if mcnc.Spec.LocalCUDN != "" {
		cudnName := mcnc.Spec.LocalCUDN
		klog.Infof("Patching existing CUDN %s with EVPN config (VNI=%d, RT=%s)",
			cudnName, mcn.Spec.VNI, mcn.Spec.RouteTarget)

		// Patch existing CUDN to add EVPN transport
		if err := r.CUDNIntegrator.PatchCUDNWithEVPN(ctx, cudnName, mcn); err != nil {
			return "", errors.Wrapf(err, "failed to patch CUDN %s with EVPN", cudnName)
		}

		return cudnName, nil
	}

	// Priority 2: CUDNSpec - SkyNet creates CUDN with EVPN (fallback - SkyNet owns network)
	if mcnc.Spec.CUDNSpec != nil {
		klog.Infof("Creating CUDN from cudnSpec with EVPN config")

		cudnName, err := r.CUDNIntegrator.CreateCUDN(ctx, mcnc.Spec.CUDNSpec, mcn)
		if err != nil {
			return "", errors.Wrap(err, "failed to create CUDN with EVPN")
		}

		return cudnName, nil
	}

	// No CUDN spec provided - error
	return "", fmt.Errorf("either localCUDN or cudnSpec must be specified")
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
	var mcnName string
	if mcnc.Spec.CreateMultiClusterNetwork != nil {
		mcnName = mcnc.Spec.CreateMultiClusterNetwork.Name
	} else if mcnc.Spec.MultiClusterNetworkName != "" {
		mcnName = mcnc.Spec.MultiClusterNetworkName
	}

	if mcnName == "" {
		// If name not set, nothing to clean up
		klog.Warningf("MCNC %s/%s has no MCN name, skipping cleanup", mcnc.Namespace, mcnc.Name)
		// Still remove finalizer to allow deletion
		if containsString(mcnc.Finalizers, mcncFinalizerName) {
			mcnc.Finalizers = removeString(mcnc.Finalizers, mcncFinalizerName)
			if err := r.Update(ctx, mcnc); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer from MCNC")
			}
		}
		return reconcile.Result{}, nil
	}

	// Determine the CUDN name
	var cudnName string
	if mcnc.Spec.CUDNSpec != nil {
		// SkyNet created the CUDN - determine its name
		cudnName = mcnc.Spec.CUDNSpec.Name
		if cudnName == "" {
			cudnName = mcnName
		}
	} else if mcnc.Spec.LocalCUDN != "" {
		// User referenced existing CUDN (not supported in POC but handle gracefully)
		cudnName = mcnc.Spec.LocalCUDN
	} else {
		// Fallback - shouldn't happen but handle gracefully
		cudnName = mcnName
	}

	// Step 1: Delete RouteAdvertisement
	if err := r.RouteAdvCreator.DeleteForCUDN(ctx, cudnName); err != nil {
		klog.Errorf("Failed to delete RouteAdvertisement for %s: %v", cudnName, err)
		// Continue cleanup even if RouteAdvertisement deletion fails
	}

	// Step 2: Delete CUDN if SkyNet created it
	if mcnc.Spec.CUDNSpec != nil {
		// SkyNet created this CUDN - delete it
		if err := r.CUDNIntegrator.DeleteCUDN(ctx, cudnName); err != nil {
			klog.Errorf("Failed to delete CUDN %s: %v", cudnName, err)
			// Continue cleanup even if CUDN deletion fails
		}
	}

	// Step 3: Leave MCN (handled by MCN manager with finalizers)
	if err := r.MCNManager.LeaveMCN(ctx, mcnName); err != nil {
		klog.Errorf("Failed to leave MCN %s: %v", mcnName, err)
		// Continue cleanup even if leave fails
	}

	// Step 4: Remove finalizer from MCNC to allow deletion
	if containsString(mcnc.Finalizers, mcncFinalizerName) {
		mcnc.Finalizers = removeString(mcnc.Finalizers, mcncFinalizerName)
		if err := r.Update(ctx, mcnc); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer from MCNC")
		}
		klog.V(2).Infof("Removed finalizer from MCNC %s/%s", mcnc.Namespace, mcnc.Name)
	}

	klog.Infof("Successfully cleaned up MCNC %s/%s", mcnc.Namespace, mcnc.Name)
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *MCNCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Note: For() automatically watches Create, Update, and Delete events
	// This includes deletionTimestamp changes
	// Previously, handleDelete failed silently due to wrong field name
	// Now fixed to read from CreateMultiClusterNetwork.Name
	return ctrl.NewControllerManagedBy(mgr).
		For(&skynetv1.MultiClusterNetworkConnect{}).
		Complete(r)
}

// containsString checks if a string is in a slice
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a slice
func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
