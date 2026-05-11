// SPDX-FileCopyrightText: Copyright The SkyNet Contributors
// SPDX-License-Identifier: Apache-2.0

package cudn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	skynetv1 "github.com/yboaron/mcn/pkg/apis/skynet.io/v1"
)

// CUDNIntegrator handles injecting EVPN configuration into existing CUDNs
type CUDNIntegrator struct {
	dynamicClient dynamic.Interface
	vtepName      string // Name of the local VTEP CR to reference
}

// Config contains configuration for CUDNIntegrator
type Config struct {
	DynamicClient dynamic.Interface
	VTEPName      string
}

// NewCUDNIntegrator creates a new CUDN integrator
func NewCUDNIntegrator(config *Config) (*CUDNIntegrator, error) {
	if config.DynamicClient == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	if config.VTEPName == "" {
		return nil, fmt.Errorf("VTEP name is required")
	}

	return &CUDNIntegrator{
		dynamicClient: config.DynamicClient,
		vtepName:      config.VTEPName,
	}, nil
}

var (
	cudnGVR = schema.GroupVersionResource{
		Group:    "k8s.ovn.org",
		Version:  "v1",
		Resource: "clusteruserdefinednetworks",
	}
)

// CreateCUDN creates a new CUDN with EVPN configuration
// This is the preferred approach for POC - SkyNet creates and owns the CUDN lifecycle
func (ci *CUDNIntegrator) CreateCUDN(ctx context.Context, cudnSpec *skynetv1.CUDNSpec, mcn *skynetv1.MultiClusterNetwork) (string, error) {
	// Determine CUDN name (use spec.name if provided, otherwise use MCN name)
	cudnName := cudnSpec.Name
	if cudnName == "" {
		cudnName = mcn.Name
	}

	klog.Infof("Creating CUDN %s with EVPN config: VNI=%d, RT=%s", cudnName, mcn.Spec.VNI, mcn.Spec.RouteTarget)

	// Check if CUDN already exists
	existing, err := ci.dynamicClient.Resource(cudnGVR).Get(ctx, cudnName, metav1.GetOptions{})
	if err == nil {
		// CUDN exists - check if it has EVPN
		if transport, found, _ := unstructured.NestedString(existing.Object, "spec", "network", "transport"); found && transport == "EVPN" {
			klog.V(4).Infof("CUDN %s already exists with EVPN transport", cudnName)
			return cudnName, nil
		}
		return "", fmt.Errorf("CUDN %s already exists without EVPN (created outside SkyNet)", cudnName)
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check if CUDN exists: %w", err)
	}

	// Build EVPN configuration
	evpnConfig := ci.buildEVPNConfigFromTopology(mcn)

	// Build network spec with EVPN
	role := cudnSpec.Role
	if role == "" {
		role = "Primary"
	}

	// Convert subnets
	var subnets []map[string]interface{}
	for _, subnet := range cudnSpec.Subnets {
		subnets = append(subnets, map[string]interface{}{
			"cidr":       subnet.CIDR,
			"hostSubnet": subnet.HostSubnet,
		})
	}

	// Build network spec with EVPN (nested under network per actual CRD schema)
	networkSpec := map[string]interface{}{
		"topology":  string(cudnSpec.Topology),
		"transport": "EVPN",
		"evpn":      evpnConfig,
	}

	if cudnSpec.Topology == "Layer3" {
		networkSpec["layer3"] = map[string]interface{}{
			"role":    role,
			"subnets": subnets,
		}
	} else if cudnSpec.Topology == "Layer2" {
		networkSpec["layer2"] = map[string]interface{}{
			"role":    role,
			"subnets": subnets,
		}
	}

	// Create CUDN object
	// NOTE: Actual CRD has transport/evpn nested under spec.network (not at top level like OKEP-5088 design)
	cudn := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.ovn.org/v1",
			"kind":       "ClusterUserDefinedNetwork",
			"metadata": map[string]interface{}{
				"name": cudnName,
			},
			"spec": map[string]interface{}{
				"namespaceSelector": map[string]interface{}{
					"matchLabels": cudnSpec.NamespaceSelector.MatchLabels,
				},
				"network": networkSpec,
			},
		},
	}

	// Create the CUDN
	klog.Infof("Creating CUDN %s (namespace selector: %v, transport: %s)", cudnName, cudnSpec.NamespaceSelector.MatchLabels, "EVPN")
	created, err := ci.dynamicClient.Resource(cudnGVR).Create(ctx, cudn, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("CUDN creation failed for %s: %v", cudnName, err)
		return "", fmt.Errorf("failed to create CUDN: %w", err)
	}
	klog.Infof("CUDN %s created successfully, UID: %s", cudnName, created.GetUID())

	// Wait for CUDN to be ready
	if err := ci.waitForCUDNReady(ctx, cudnName); err != nil {
		klog.Warningf("CUDN %s created but NetworkCreated condition not met: %v", cudnName, err)
		// Don't fail - CUDN exists even if condition check times out
	}

	klog.Infof("Successfully created CUDN %s with EVPN config", cudnName)
	return cudnName, nil
}

// PatchCUDNWithEVPN patches an existing CUDN to add EVPN transport configuration
// This is used when the user has pre-created a CUDN and SkyNet adds EVPN connectivity
func (ci *CUDNIntegrator) PatchCUDNWithEVPN(ctx context.Context, cudnName string, mcn *skynetv1.MultiClusterNetwork) error {
	klog.Infof("Patching CUDN %s with EVPN config: VNI=%d, RT=%s", cudnName, mcn.Spec.VNI, mcn.Spec.RouteTarget)

	// Get existing CUDN
	cudn, err := ci.dynamicClient.Resource(cudnGVR).Get(ctx, cudnName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("CUDN %s not found: %w", cudnName, err)
	}

	// Validate CUDN is compatible with EVPN
	if err := ci.validateCUDN(cudn, mcn); err != nil {
		return fmt.Errorf("CUDN validation failed: %w", err)
	}

	// Check if already has EVPN transport
	if transport, found, _ := unstructured.NestedString(cudn.Object, "spec", "network", "transport"); found && transport == "EVPN" {
		klog.V(4).Infof("CUDN %s already has EVPN transport, checking config", cudnName)

		// Verify VNI and RT match
		existingVNI, _, _ := unstructured.NestedInt64(cudn.Object, "spec", "network", "evpn", "ipVRF", "vni")
		existingRT, _, _ := unstructured.NestedString(cudn.Object, "spec", "network", "evpn", "ipVRF", "routeTarget")

		if uint32(existingVNI) == mcn.Spec.VNI && existingRT == mcn.Spec.RouteTarget {
			klog.Infof("CUDN %s already has matching EVPN config, no patch needed", cudnName)
			return nil
		}

		klog.Warningf("CUDN %s has different EVPN config (VNI=%d vs %d, RT=%s vs %s), updating",
			cudnName, existingVNI, mcn.Spec.VNI, existingRT, mcn.Spec.RouteTarget)
	}

	// Build EVPN config
	evpnConfig := ci.buildEVPNConfigFromTopology(mcn)

	// Prepare patch to add transport + evpn config
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"network": map[string]interface{}{
				"transport": "EVPN",
				"evpn":      evpnConfig,
			},
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	klog.V(4).Infof("Patching CUDN %s with: %s", cudnName, string(patchBytes))

	// Apply patch (merge patch)
	_, err = ci.dynamicClient.Resource(cudnGVR).Patch(ctx, cudnName,
		types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch CUDN: %w", err)
	}

	klog.Infof("Successfully patched CUDN %s with EVPN config (VNI=%d, RT=%s)",
		cudnName, mcn.Spec.VNI, mcn.Spec.RouteTarget)

	return nil
}

// validateCUDN ensures the CUDN is compatible with EVPN stretching
func (ci *CUDNIntegrator) validateCUDN(cudn *unstructured.Unstructured, mcn *skynetv1.MultiClusterNetwork) error {
	// Get spec.network
	network, found, err := unstructured.NestedMap(cudn.Object, "spec", "network")
	if err != nil || !found {
		return fmt.Errorf("CUDN spec.network not found")
	}

	// Check if already has transport set
	if transport, found, _ := unstructured.NestedString(cudn.Object, "spec", "network", "transport"); found && transport != "" {
		// Allow if already set to EVPN
		if transport != "EVPN" {
			return fmt.Errorf("CUDN already has transport=%s (must be unset or EVPN)", transport)
		}
		klog.V(4).Infof("CUDN already has transport=EVPN, will update EVPN config")
	}

	// Validate topology matches MCN
	topology, found, err := unstructured.NestedString(network, "topology")
	if err != nil || !found {
		return fmt.Errorf("CUDN topology not found")
	}

	if skynetv1.NetworkTopology(topology) != mcn.Spec.Topology {
		return fmt.Errorf("CUDN topology (%s) does not match MCN topology (%s)", topology, mcn.Spec.Topology)
	}

	klog.V(4).Infof("CUDN validation passed: topology=%s", topology)
	return nil
}

// buildEVPNConfigFromTopology constructs EVPN configuration based on MCN topology
func (ci *CUDNIntegrator) buildEVPNConfigFromTopology(mcn *skynetv1.MultiClusterNetwork) map[string]interface{} {
	evpnConfig := map[string]interface{}{
		"vtep": ci.vtepName,
	}

	// Build VRF config based on topology
	switch mcn.Spec.Topology {
	case "Layer3":
		// Layer3 requires ipVRF
		evpnConfig["ipVRF"] = map[string]interface{}{
			"vni":         mcn.Spec.VNI,
			"routeTarget": mcn.Spec.RouteTarget,
		}
		klog.V(4).Infof("Built Layer3 EVPN config: ipVRF with VNI=%d, RT=%s", mcn.Spec.VNI, mcn.Spec.RouteTarget)

	case "Layer2":
		// Layer2 requires macVRF
		evpnConfig["macVRF"] = map[string]interface{}{
			"vni":         mcn.Spec.VNI,
			"routeTarget": mcn.Spec.RouteTarget,
		}
		klog.V(4).Infof("Built Layer2 EVPN config: macVRF with VNI=%d, RT=%s", mcn.Spec.VNI, mcn.Spec.RouteTarget)
	}

	return evpnConfig
}


// DeleteCUDN deletes a CUDN created by SkyNet
// This is used during MCNC cleanup when SkyNet created the CUDN
func (ci *CUDNIntegrator) DeleteCUDN(ctx context.Context, cudnName string) error {
	klog.Infof("Deleting CUDN %s (created by SkyNet)", cudnName)

	// Delete the CUDN
	err := ci.dynamicClient.Resource(cudnGVR).Delete(ctx, cudnName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("CUDN %s not found, already deleted", cudnName)
			return nil
		}
		return fmt.Errorf("failed to delete CUDN: %w", err)
	}

	klog.Infof("Successfully deleted CUDN %s", cudnName)
	return nil
}

// waitForCUDNDeletion waits for CUDN to be deleted (not found)
func (ci *CUDNIntegrator) waitForCUDNDeletion(ctx context.Context, cudnName string) error {
	// Poll for up to 30 seconds
	for i := 0; i < 30; i++ {
		_, err := ci.dynamicClient.Resource(cudnGVR).Get(ctx, cudnName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("error checking CUDN: %w", err)
		}
		// CUDN still exists, wait 1 second
		klog.V(4).Infof("Waiting for CUDN %s deletion (attempt %d/30)", cudnName, i+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("timeout waiting for CUDN deletion")
}

// waitForCUDNReady waits for CUDN NetworkCreated condition
func (ci *CUDNIntegrator) waitForCUDNReady(ctx context.Context, cudnName string) error {
	// Poll for up to 60 seconds
	for i := 0; i < 60; i++ {
		cudn, err := ci.dynamicClient.Resource(cudnGVR).Get(ctx, cudnName, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("error getting CUDN: %w", err)
			}
			// CUDN not found yet, continue waiting
		} else {
			// Check for NetworkCreated condition
			conditions, found, _ := unstructured.NestedSlice(cudn.Object, "status", "conditions")
			if found {
				for _, cond := range conditions {
					condMap, ok := cond.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _, _ := unstructured.NestedString(condMap, "type")
					condStatus, _, _ := unstructured.NestedString(condMap, "status")
					if condType == "NetworkCreated" && condStatus == "True" {
						klog.V(4).Infof("CUDN %s is ready (NetworkCreated=True)", cudnName)
						return nil
					}
				}
			}
		}

		klog.V(4).Infof("Waiting for CUDN %s to be ready (attempt %d/60)", cudnName, i+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("timeout waiting for CUDN to be ready")
}
