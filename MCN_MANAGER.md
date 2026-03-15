# MultiClusterNetwork Manager Implementation

## Overview

The MultiClusterNetwork (MCN) Manager handles the lifecycle of MultiClusterNetwork resources on the broker cluster. It implements the create-or-join pattern with finalizer-based lifecycle management.

## Location

- **Implementation**: `pkg/agent/mcn/mcn_manager.go`
- **Tests**: `pkg/agent/mcn/mcn_manager_test.go`

## Key Concepts

### Create vs Join

When a cluster wants to connect to a MultiClusterNetwork:

1. **Join Existing MCN**: If MCN already exists on broker
   - Add cluster's finalizer to MCN
   - Use existing VNI and RouteTarget

2. **Create New MCN**: If MCN doesn't exist
   - Allocate new VNI using VNI allocator
   - Generate RouteTarget (format: `65000:<VNI>`)
   - Create MCN on broker with cluster's finalizer

### Finalizer Management

Finalizers track which clusters are using an MCN:

**Format**: `<clusterID>.multicluster.ovn.org`

**Example**:
```yaml
apiVersion: multicluster.ovn.org/v1alpha1
kind: MultiClusterNetwork
metadata:
  name: tenant-alpha
  finalizers:
  - cluster-east.multicluster.ovn.org
  - cluster-west.multicluster.ovn.org
spec:
  vni: 5001
  routeTarget: "65000:5001"
  topology: Layer3
```

**Purpose**:
- Prevent MCN deletion while clusters are using it
- Track cluster membership
- Enable automatic cleanup when last cluster leaves

## API

### Main Functions

```go
// NewMCNManager creates a new MCN manager
func NewMCNManager(config *Config) (*MCNManager, error)

// CreateOrJoinMCN creates or joins a MultiClusterNetwork
// Returns MCN with VNI and RouteTarget populated
func (m *MCNManager) CreateOrJoinMCN(ctx context.Context, mcnName string, topology skynetv1.NetworkTopology) (*skynetv1.MultiClusterNetwork, error)

// LeaveMCN removes this cluster's finalizer from MCN
func (m *MCNManager) LeaveMCN(ctx context.Context, mcnName string) error

// GetMCN retrieves an MCN from the broker
func (m *MCNManager) GetMCN(ctx context.Context, mcnName string) (*skynetv1.MultiClusterNetwork, error)
```

### Configuration

```go
type Config struct {
    BrokerClient dynamic.Interface       // Broker cluster client
    BrokerNS     string                   // Broker namespace
    ClusterID    string                   // This cluster's ID
    VNIAllocator *allocator.VNIAllocator  // VNI allocator
}
```

## Workflows

### Workflow 1: Create New MCN

```
Agent (cluster-east)
    |
    | CreateOrJoinMCN("tenant-alpha", Layer3)
    v
Check if MCN exists on broker
    |
    | Not found
    v
Allocate VNI: 5001
    |
    v
Generate RT: "65000:5001"
    |
    v
Create MCN on broker:
  - name: tenant-alpha
  - finalizers: [cluster-east.multicluster.ovn.org]
  - vni: 5001
  - routeTarget: "65000:5001"
  - topology: Layer3
    |
    v
Return MCN to caller
```

### Workflow 2: Join Existing MCN

```
Agent (cluster-west)
    |
    | CreateOrJoinMCN("tenant-alpha", Layer3)
    v
Check if MCN exists on broker
    |
    | Found: vni=5001, rt="65000:5001"
    v
Check if our finalizer exists
    |
    | Not found
    v
Add finalizer: cluster-west.multicluster.ovn.org
    |
    v
Update MCN on broker:
  - finalizers: [
      cluster-east.multicluster.ovn.org,
      cluster-west.multicluster.ovn.org
    ]
    |
    v
Return MCN to caller
```

### Workflow 3: Race Condition Handling

```
Agent cluster-east          Agent cluster-west
    |                            |
    | CreateOrJoinMCN            | CreateOrJoinMCN
    v                            v
MCN not found               MCN not found
    |                            |
Allocate VNI: 5001          Allocate VNI: 5001
    |                            |
Create MCN                  Create MCN
    |                            |
SUCCESS ✓                   CONFLICT (AlreadyExists)
    |                            |
    |                        Retry: Get MCN
    |                            |
    |                        Join existing MCN ✓
    v                            v
```

**Handling**: Built-in race condition handling via optimistic locking
- First cluster to create wins
- Second cluster automatically joins the created MCN

### Workflow 4: Leave MCN

```
Agent (cluster-east)
    |
    | LeaveMCN("tenant-alpha")
    v
Get MCN from broker
    |
    v
Remove finalizer: cluster-east.multicluster.ovn.org
    |
    v
Update MCN on broker:
  - finalizers: [cluster-west.multicluster.ovn.org]
    |
    v
Done

If last finalizer removed → K8s can GC the MCN
```

## Usage Example

```go
// Initialize MCN manager
vniAllocator := allocator.NewVNIAllocator(brokerClient, "skynet-broker", "cluster-east")

mcnManager, err := mcn.NewMCNManager(&mcn.Config{
    BrokerClient: brokerClient,
    BrokerNS:     "skynet-broker",
    ClusterID:    "cluster-east",
    VNIAllocator: vniAllocator,
})
if err != nil {
    return err
}

// Create or join MCN
mcn, err := mcnManager.CreateOrJoinMCN(ctx, "tenant-alpha", skynetv1.NetworkTopologyLayer3)
if err != nil {
    return err
}

// Now use MCN.Spec.VNI and MCN.Spec.RouteTarget
fmt.Printf("MCN VNI: %d, RT: %s\n", mcn.Spec.VNI, mcn.Spec.RouteTarget)
// Output: MCN VNI: 5001, RT: 65000:5001

// Later, when disconnecting...
err = mcnManager.LeaveMCN(ctx, "tenant-alpha")
```

## Integration with MCNC Controller

The MCN manager will be used by the MultiClusterNetworkConnect controller:

```go
// In MCNC reconciler
func (r *MCNCReconciler) reconcileMCNC(ctx context.Context, mcnc *skynetv1.MultiClusterNetworkConnect) error {
    // Determine MCN name
    mcnName := mcnc.Spec.MultiClusterNetworkName
    if mcnName == "" && mcnc.Spec.CreateMultiClusterNetwork != nil {
        mcnName = mcnc.Spec.CreateMultiClusterNetwork.Name
    }

    // Determine topology
    topology := skynetv1.NetworkTopologyLayer3 // default
    if mcnc.Spec.CreateMultiClusterNetwork != nil {
        topology = mcnc.Spec.CreateMultiClusterNetwork.Topology
    }

    // Create or join MCN
    mcn, err := r.mcnManager.CreateOrJoinMCN(ctx, mcnName, topology)
    if err != nil {
        return err
    }

    // Update MCNC status
    mcnc.Status.VNI = mcn.Spec.VNI
    mcnc.Status.RouteTarget = mcn.Spec.RouteTarget

    // Continue with CUDN integration...
}
```

## Design Decisions

### Why Finalizers?

**Benefits**:
- **Lifecycle Tracking**: Know which clusters are using an MCN
- **Automatic Cleanup**: Last cluster leaving allows K8s to delete MCN
- **Race Condition Safe**: Multiple clusters can safely join/leave concurrently
- **Kubernetes Native**: Uses standard K8s garbage collection

**Alternative Considered**:
- Manual cleanup via agent deletion hooks
- Rejected: More complex, less reliable, non-standard

### Why Create-or-Join Pattern?

**Benefits**:
- **Idempotent**: Safe to call multiple times
- **Race Condition Safe**: First cluster creates, others join
- **Simple API**: Single function handles both cases
- **Matches User Intent**: User doesn't care if MCN exists or not

**Matches HLD**:
- Section 4.B.1: "Create MultiClusterNetworkConnect on cluster-east" with `createMultiClusterNetwork`
- Section 4.B.1: "Join multi-cluster network on cluster-west" with `multiClusterNetworkName`

### Why Optimistic Locking?

**Benefits**:
- **No Central Coordinator**: Broker is pure storage
- **Conflict Resolution**: K8s API server handles conflicts
- **Retry Logic**: Built-in retry on conflict
- **Proven Pattern**: Same pattern used throughout SkyNet (VTEP, ASN, VNI)

## Error Handling

### Conflict Errors

**Scenario**: Two agents try to create same MCN simultaneously

**Handling**: Automatic retry with join logic
```go
if apierrors.IsAlreadyExists(err) {
    existingMCN, _ := m.GetMCN(ctx, mcnName)
    return m.joinExistingMCN(ctx, existingMCN)
}
```

### Not Found Errors

**Scenario**: LeaveMCN called on non-existent MCN

**Handling**: Silently succeed (idempotent)
```go
if apierrors.IsNotFound(err) {
    return nil // Already gone
}
```

## Testing

### Unit Tests (All Passing ✓)

- ✅ Finalizer name generation
- ✅ String slice helpers (contains, remove)
- ✅ Configuration validation
- ✅ Edge cases (empty slices, duplicates, etc.)

### Integration Tests (To Do)

Will be tested with full agent deployment:
1. Create MCN from first cluster
2. Join MCN from second cluster
3. Verify finalizers on broker
4. Leave from first cluster
5. Verify finalizer removed
6. Leave from second cluster
7. Verify MCN deleted by K8s GC

## Alignment with HLD

This implementation follows the HLD design:

✅ **Section 3.2 - MultiClusterNetwork**:
- Finalizers track connected clusters ✓
- VNI allocation on creation ✓
- Route target generation ✓

✅ **Section 4.B.1 - Layer3 CUDN Stretching**:
- Create MCN when first cluster connects ✓
- Join MCN when additional clusters connect ✓
- Automatic VNI/RT allocation ✓

✅ **Architecture Principles**:
- Agent-side logic (broker is storage) ✓
- Optimistic locking for conflicts ✓
- Kubernetes-native patterns (finalizers) ✓

## Next Steps

1. ✅ **VNI Allocator** - COMPLETE
2. ✅ **MultiClusterNetwork Manager** - COMPLETE
3. ⏭️ **CUDN Integrator** - Inject VNI/RT into local CUDN
4. ⏭️ **Update MCNC Controller** - Integrate MCN manager
5. ⏭️ **RouteAdvertisement Manager** - Create RouteAdvertisement CRs

Would you like me to proceed with the CUDN Integrator next?
