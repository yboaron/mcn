# VTEP CRD Compatibility Issue

## Problem

Our VTEP manager code doesn't match the actual OVN-K VTEP CRD specification.

### OVN-K VTEP CRD (Actual Spec)

```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: cluster-vtep
spec:
  cidrs: ["100.0.0.0/16"]  # CIDR for VTEP IP allocation
  mode: Managed              # or Unmanaged
```

**Purpose**: Configure VTEP IP allocation for EVPN on this cluster.

- `cidrs`: IP ranges from which VTEP IPs are allocated to nodes
- `mode`:
  - `Managed`: OVN-K allocates VTEP IPs automatically
  - `Unmanaged`: External provider handles IP assignment

### Our Code (Current Implementation)

**File**: `pkg/agent/vtep/vtep_manager.go:106-109`

```go
vtepSpec := map[string]interface{}{
    "name": vtepName,
    "endpoints": m.buildVtepEndpoints(remoteCluster),  // NOT in OVN-K spec!
}
```

**What we're trying to create**:
```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: skynet-cluster2
spec:
  name: skynet-cluster2      # ✗ Not in OVN-K spec
  endpoints:                  # ✗ Not in OVN-K spec
  - node: node1
    ip: 192.168.2.10
    vtepIP: 100.1.0.1
```

**Purpose**: Store remote cluster endpoint information for BGP peering.

## Root Cause

We're conflating two different concerns:

1. **VTEP IP Allocation** (what OVN-K VTEP CRD does)
   - Allocates VTEP IPs for local nodes
   - Used by OVN for VXLAN tunnel endpoints

2. **Remote Cluster Endpoints** (what we need for BGP)
   - Store remote cluster node IPs for BGP peering
   - Track VTEP IPs assigned to remote nodes

## Solutions

### Option 1: Use OVN-K VTEP CRD Correctly (Recommended)

**For Local Cluster**:
Create VTEP CR on local cluster to allocate VTEP IPs for local nodes:

```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: local-vtep
spec:
  cidrs: ["100.0.0.0/16"]  # Our allocated VTEP CIDR
  mode: Managed
```

OVN-K will then:
- Allocate VTEP IPs to nodes (100.0.0.1, 100.0.0.2, ...)
- Create node annotations or status with allocated IPs
- Configure VXLAN tunnels

**For Remote Cluster Info**:
Don't create VTEP CRs for remote clusters. Instead:
- Get remote node info from Cluster CR on broker
- Use that info directly for BGP configuration
- No need to store it locally in a VTEP CR

### Option 2: Create Custom CRD for Cluster Endpoints

Create our own `ClusterEndpoints` CRD:

```yaml
apiVersion: skynet.io/v1
kind: ClusterEndpoints
metadata:
  name: cluster2
spec:
  clusterID: cluster2
  endpoints:
  - node: node1
    bgpPeerIP: 192.168.2.10
    vtepIP: 100.1.0.1
  - node: node2
    bgpPeerIP: 192.168.2.11
    vtepIP: 100.1.0.2
```

But this is **redundant** since we already have this info in the Cluster CR on the broker!

### Option 3: Use Broker Cluster CR Only (Simplest)

**Don't create local VTEP CRs for remote clusters at all!**

- Remote cluster info is already in the Cluster CR on broker
- BGP configurator can read from broker directly
- VTEP manager only creates local VTEP for local nodes

## Recommended Approach

**Use Option 3** - Simplify the architecture:

### What Changes

1. **VTEP Manager** (`pkg/agent/vtep/vtep_manager.go`)
   - Create ONE local VTEP CR for this cluster
   - Spec: `{cidrs: [localVtepCIDR], mode: "Managed"}`
   - Do NOT create VTEPs for remote clusters

2. **BGP Configurator** (`pkg/agent/bgp/bgp_configurator.go`)
   - Read remote cluster endpoints from Cluster CRs on broker (already have this!)
   - Build BGP neighbors from `cluster.Status.Endpoints`
   - No need to read VTEPs

3. **Agent Reconciliation** (`pkg/agent/agent.go`)
   - Create local VTEP once during initialization
   - BGP configurator already gets remote clusters from broker syncer

### Benefits

- ✅ Uses OVN-K VTEP CRD correctly
- ✅ Simpler architecture (one source of truth: broker)
- ✅ No duplicate data storage
- ✅ Fewer CRs to reconcile

### Implementation Plan

```go
// In VTEP Manager
func (m *VtepManager) EnsureLocalVTEP(ctx context.Context) error {
    vtep := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "k8s.ovn.org/v1",
            "kind":       "VTEP",
            "metadata": map[string]interface{}{
                "name": "skynet-local",
            },
            "spec": map[string]interface{}{
                "cidrs": []string{m.vtepCIDR},
                "mode":  "Managed",
            },
        },
    }

    // Create or update
    // ...
}

// Remove ReconcileVteps() - not needed anymore
```

```go
// In Agent
func (a *Agent) initRuntimeComponents() error {
    // ...

    // Create local VTEP once
    if err := a.vtepManager.EnsureLocalVTEP(ctx); err != nil {
        return err
    }

    // ...
}

// In reconcile()
func (a *Agent) reconcile(ctx context.Context) error {
    // ...

    // Get remote clusters from broker (already doing this!)
    remoteClusters, err := a.brokerSyncer.GetRemoteClusters(ctx)

    // Skip VTEP reconciliation - don't need it!

    // BGP configurator uses remoteClusters directly
    if err := a.bgpConfigurator.ReconcileBGPConfig(ctx, remoteClusters, multiClusterNetworks); err != nil {
        return err
    }
}
```

## Testing Impact

### Before Fix

```bash
kubectl --context kind-cluster1 get vteps
# Expected: skynet-cluster2 (WRONG - doesn't match OVN-K spec!)
```

### After Fix

```bash
kubectl --context kind-cluster1 get vteps
# Expected: skynet-local (correct OVN-K VTEP for local cluster)

# Remote cluster info comes from broker:
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml
# Shows cluster2 endpoints
```

## Next Steps

1. **Fix VTEP Manager**: Create only local VTEP with correct spec
2. **Update Agent**: Remove VTEP reconciliation for remote clusters
3. **Test**: Verify VTEP CR conforms to OVN-K spec
4. **Verify**: OVN-K allocates VTEP IPs to local nodes
5. **BGP**: Confirm BGP config still works using broker data

## References

- OVN-K VTEP CRD: `go-controller/pkg/crd/vtep/v1/types.go`
- OVN-K VTEP YAML: `helm/ovn-kubernetes/crds/k8s.ovn.org_vteps.yaml`
- OKEP-5088: EVPN Support
