# VTEP Manager Fix - Summary

## Changes Made

Fixed VTEP manager to use the correct OVN-K VTEP CRD specification.

### Before (Incorrect)

**Created VTEPs for remote clusters with invalid spec**:
```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: skynet-cluster2  # Remote cluster
spec:
  name: skynet-cluster2      # ✗ Not in OVN-K spec
  endpoints:                  # ✗ Not in OVN-K spec
  - node: node1
    ip: 192.168.2.10
    vtepIP: 100.1.0.1
```

**Problems**:
- Spec doesn't match OVN-K VTEP CRD
- Created VTEPs for remote clusters (wrong!)
- OVN-K couldn't allocate VTEP IPs
- Duplicated remote cluster data already on broker

### After (Correct)

**Creates ONE local VTEP with correct OVN-K spec**:
```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: skynet-local  # Local cluster only
spec:
  cidrs: ["100.0.0.0/16"]  # ✓ OVN-K spec
  mode: Managed             # ✓ OVN-K manages IP allocation
```

**Benefits**:
- Conforms to OVN-K VTEP CRD spec
- OVN-K allocates VTEP IPs to local nodes automatically
- No duplicate data - remote cluster info comes from broker
- Simpler architecture - one source of truth (broker)

## Files Modified

### 1. pkg/agent/vtep/vtep_manager.go

**Removed**:
- `ReconcileVteps()` - No longer creating VTEPs for remote clusters
- `reconcileVtepForCluster()` - Not needed
- `buildVtepEndpoints()` - Not needed
- `DeleteVtep()` - Replaced with `DeleteLocalVTEP()`
- `BrokerClient` and `BrokerNS` from Config - Not needed

**Added**:
- `EnsureLocalVTEP()` - Creates/updates local VTEP with correct spec
- `DeleteLocalVTEP()` - Deletes local VTEP
- `LocalVTEPName` constant - "skynet-local"
- `VTEPModeManaged` constant - "Managed"

**Changed**:
- VTEP spec now uses `cidrs` and `mode` (OVN-K spec)
- Manager only handles local VTEP
- Simplified Config struct

### 2. pkg/agent/agent.go

**Added**:
- Call to `EnsureLocalVTEP()` in `Start()` method
- Creates local VTEP once during agent startup

**Removed**:
- `ReconcileVteps()` call in `reconcile()` method
- BrokerClient/BrokerNS from VtepManager config

**Changed**:
- BGP configurator uses remote cluster data from broker directly
- Added comments explaining why VTEP reconciliation removed

### 3. pkg/agent/endpoint/endpoint_reporter.go

**Changed**:
- `CollectEndpoints()` signature: `vtepIPAllocator func() (string, error)` → `func(string) (string, error)`
- Now passes node name to allocator: `vtepIPAllocator(node.Name)`

**Reason**: VtepManager.AllocateVtepIP needs node name for logging

## How It Works Now

### Agent Startup

```
1. Register cluster with broker
   └─ Allocate VTEP CIDR (e.g., 100.0.0.0/16)
   └─ Allocate ASN (e.g., 64512)

2. Initialize runtime components
   └─ Create VTEP manager with local VTEP CIDR

3. Create local VTEP CR
   └─ Spec: {cidrs: ["100.0.0.0/16"], mode: "Managed"}
   └─ OVN-K sees this and starts allocating VTEP IPs to nodes

4. Start reconciliation
```

### Reconciliation Loop

```
1. Collect local node endpoints
   └─ For each node: get BGP peer IP, allocate VTEP IP
   └─ Report to broker Cluster CR

2. Get remote clusters from broker
   └─ broker syncer fetches Cluster CRs
   └─ Each Cluster CR has Status.Endpoints with node info

3. Generate BGP configuration
   └─ Build FRRConfiguration with neighbors from remote cluster endpoints
   └─ No need for local VTEP CRs - data comes from broker
```

## VTEP Resource Comparison

### Before Fix

```bash
kubectl get vteps

NAME              READY
skynet-local      True   # Created by agent (WRONG spec)
skynet-cluster2   False  # ✗ For remote cluster (shouldn't exist!)
skynet-cluster3   False  # ✗ For remote cluster (shouldn't exist!)
```

### After Fix

```bash
kubectl get vteps

NAME            READY
skynet-local    True   # ✓ Created by agent (CORRECT spec)
                       # OVN-K allocates IPs: 100.0.0.1, 100.0.0.2, ...
```

## Data Flow

### Remote Cluster Endpoint Info

**Before**: Stored in local VTEP CRs (WRONG)

**After**: Comes from broker Cluster CRs (CORRECT)

```
Broker (cluster1):
  Cluster CR: cluster2
    Status:
      Endpoints:
      - node: node1
        bgpPeerIP: 192.168.2.10
        vtepIP: 100.1.0.1
      - node: node2
        bgpPeerIP: 192.168.2.11
        vtepIP: 100.1.0.2

        ↓ (broker syncer fetches)

Agent (cluster1):
  BGP Configurator reads endpoints directly
  Generates FRRConfiguration with neighbors:
  - 192.168.2.10 (cluster2 node1)
  - 192.168.2.11 (cluster2 node2)
```

## Testing Impact

### What to Check

1. **VTEP CR Created**:
   ```bash
   kubectl get vtep skynet-local -o yaml
   ```

   Expected spec:
   ```yaml
   spec:
     cidrs: ["100.0.0.0/16"]
     mode: Managed
   ```

2. **OVN-K Allocates VTEP IPs**:
   ```bash
   # Check node annotations or VTEP status
   kubectl describe vtep skynet-local
   ```

   Should show VTEP IPs allocated to nodes

3. **No Remote Cluster VTEPs**:
   ```bash
   kubectl get vteps
   ```

   Should only show `skynet-local`

4. **BGP Config Still Works**:
   ```bash
   kubectl get frrconfiguration skynet-bgp-config -o yaml
   ```

   Should have neighbors from remote clusters

5. **Broker Has Cluster Info**:
   ```bash
   kubectl --context broker -n skynet-broker get cluster cluster1 -o yaml
   ```

   Should show Status.Endpoints with node info

## Build Verification

```bash
# Build agent package
go build ./pkg/agent/...

# Build agent binary
go build -o bin/skynet-agent ./cmd/agent

# Both completed successfully ✓
```

## Next Steps

1. **Test with new setup script**:
   ```bash
   cd test
   ./setup-clusters-v2.sh  # Uses GitHub OVN-K
   ```

2. **Deploy agent**:
   ```bash
   ./build-and-load.sh
   ./deploy-agents.sh
   ```

3. **Verify VTEP creation**:
   ```bash
   kubectl --context kind-cluster1 get vtep skynet-local -o yaml
   kubectl --context kind-cluster2 get vtep skynet-local -o yaml
   ```

4. **Check BGP peering**:
   ```bash
   ./verify-bgp.sh
   ```

## References

- **Issue Documentation**: `VTEP_CRD_ISSUE.md`
- **OVN-K VTEP Spec**: `go-controller/pkg/crd/vtep/v1/types.go`
- **OKEP-5088**: OVN-K EVPN Support
- **Test Setup**: `test/setup-clusters-v2.sh`
