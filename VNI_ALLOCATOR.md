# VNI Allocator Implementation

## Overview

The VNI (VXLAN Network Identifier) allocator is responsible for allocating unique VNIs for MultiClusterNetwork resources. It follows the same optimistic locking pattern as the existing VTEP and ASN allocators.

## Location

- **Implementation**: `pkg/agent/allocator/vni_allocator.go`
- **Tests**: `pkg/agent/allocator/vni_allocator_test.go`

## Key Features

### 1. VNI Range Management
- **Range**: 5000-10000 (5,001 available VNIs)
- **Allocation**: Sequential from 5000 upward
- **Collision Avoidance**: Optimistic locking - checks broker for existing allocations

### 2. Route Target Generation
- **Fixed ASN**: 65000 (as per HLD)
- **Format**: `<ASN>:<VNI>` (e.g., "65000:5001")
- **Function**: `GenerateRouteTarget(vni uint32) string`

### 3. Optimistic Locking Pattern
Matches the pattern used by VTEP and ASN allocators:
1. Query broker for all MultiClusterNetwork resources
2. Extract allocated VNIs
3. Find first available VNI in range
4. Return to caller (caller handles create/update on broker)

## API

### Main Functions

```go
// NewVNIAllocator creates a new VNI allocator
func NewVNIAllocator(brokerClient dynamic.Interface, brokerNS, clusterID string) *VNIAllocator

// AllocateVNI allocates a VNI for a MultiClusterNetwork
// Returns the allocated VNI or error
func (a *VNIAllocator) AllocateVNI(ctx context.Context, mcn *skynetv1.MultiClusterNetwork) (uint32, error)

// GenerateRouteTarget generates RT string from VNI
// Format: "65000:<VNI>"
func GenerateRouteTarget(vni uint32) string

// ValidateVNI validates a VNI is in valid range [5000-10000]
func ValidateVNI(vni uint32) error
```

### Constants

```go
const (
    VNIMin         = 5000   // Minimum VNI
    VNIMax         = 10000  // Maximum VNI
    RouteTargetASN = 65000  // Fixed ASN for route targets
)
```

## Usage Example

```go
// Create VNI allocator
vniAllocator := allocator.NewVNIAllocator(brokerClient, "skynet-broker", "cluster-east")

// Allocate VNI for a MultiClusterNetwork
mcn := &skynetv1.MultiClusterNetwork{
    ObjectMeta: metav1.ObjectMeta{
        Name: "tenant-alpha",
    },
}

vni, err := vniAllocator.AllocateVNI(ctx, mcn)
if err != nil {
    return err
}

// Generate route target
rt := allocator.GenerateRouteTarget(vni)

// Update MCN spec
mcn.Spec.VNI = vni
mcn.Spec.RouteTarget = rt
mcn.Spec.Topology = skynetv1.NetworkTopologyLayer3

// Create/update on broker
// ... (caller's responsibility)
```

## Integration Points

### Next Implementation Steps

The VNI allocator will be used by:

1. **MultiClusterNetwork Manager** (to be implemented)
   - Use VNI allocator when creating new MCN
   - Handle both create and join scenarios
   - Manage MCN lifecycle with finalizers

2. **MCNC Controller Updates** (to be implemented)
   - Check if MCN exists (join) or needs creation
   - Use VNI allocator for new MCNs
   - Update MCN on broker with allocated VNI/RT

3. **CUDN Integrator** (to be implemented)
   - Fetch VNI/RT from MCN
   - Inject into local CUDN spec
   - Create RouteAdvertisement CRs

## Design Decisions

### Why Fixed ASN 65000?

Per the HLD, using a fixed ASN (65000) for route targets simplifies management:
- **Global VNI Pool**: VNI uniqueness is sufficient across all clusters
- **No ASN Coordination**: Don't need to coordinate per-cluster ASNs for route targets
- **Simpler Configuration**: Route target is always `65000:<VNI>`

This differs from the per-cluster ASN allocation (64512-65534) used for BGP peering.

### Why Sequential Allocation?

- **Simplicity**: Easy to understand and debug
- **Predictable**: VNIs allocated in order
- **Gap Filling**: Automatically fills gaps from deleted MCNs
- **Sufficient**: 5,001 VNIs is more than enough for multi-cluster networks

### Why Optimistic Locking?

- **No Central Controller**: Broker is pure storage (no controllers)
- **Agent-Side Logic**: All allocation happens in agents
- **Conflict Resolution**: If two agents try to allocate same VNI, one will fail on create and retry
- **Proven Pattern**: Same pattern used by Submariner and already implemented for VTEP/ASN

## Testing

### Unit Tests

All tests pass (see `vni_allocator_test.go`):
- ✅ VNI validation (range checking)
- ✅ Route target generation
- ✅ Next available VNI finding
- ✅ Gap filling in allocated VNIs

### Integration Testing

Will be tested with full agent deployment:
1. Deploy agents on 2 clusters
2. Create MultiClusterNetworkConnect
3. Verify VNI allocated in range
4. Verify route target format
5. Verify no collisions with multiple MCNs

## Alignment with HLD

This implementation follows the HLD design:

✅ **Section 3.2 (Broker Resources) - MultiClusterNetwork**:
- VNI allocation from pool (5000-10000) ✓
- Route target format `<ASN>:<VNI>` ✓
- Fixed ASN 65000 for route targets ✓

✅ **Section 4.B.1 (Layer3 CUDN Stretching)**:
- Allocates VNI (5002) automatically ✓
- Generates RT (65000:5002) automatically ✓

✅ **Architecture Principle - "Agent Does Everything"**:
- All allocation logic in agent ✓
- Broker is pure storage ✓
- Optimistic locking for conflict resolution ✓

## Next Steps

1. ✅ **VNI Allocator** - COMPLETE
2. ⏭️ **MultiClusterNetwork Manager** - Create/join logic, finalizers
3. ⏭️ **CUDN Integrator** - Inject VNI/RT, create RouteAdvertisements
4. ⏭️ **Update MCNC Controller** - Orchestrate the workflow
5. ⏭️ **Route Reflector Topology** - Implement in BGP configurator

Would you like me to proceed with the MultiClusterNetwork Manager next?
