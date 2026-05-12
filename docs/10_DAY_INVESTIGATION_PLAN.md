# 10-Day Investigation Plan for OKEP
**Period:** May 12-22, 2026  
**Goal:** Complete high-priority research to support OKEP draft

---

## Overview

This is a focused investigation plan covering the **critical research needed** for the OKEP. All tasks are research/investigation - no implementation yet.

**Total:** 3 high-priority investigation tasks (~12 days of work compressed into 10 days with some parallel work)

---

## Day 1-3: CUDN Topology Deep Dive 🔴

**Task:** Understand all CUDN topology types and stretching feasibility

### Investigation Questions
1. What are the differences between `layer2`, `layer3`, and `localnet` topologies?
2. How does OVN-K implement each internally? (OVN logical switches vs routers)
3. Which topologies can be stretched across clusters?
4. What transport requirements does each have? (EVPN Type-2 vs Type-5, GRE L2 vs L3)

### Daily Breakdown

**Day 1 (Mon):** Documentation Review
```bash
# Read OVN-K CUDN documentation
cat ovn-kubernetes/docs/user-defined-networks.md
cat ovn-kubernetes/docs/design/user-defined-networks.md

# Review CRD definitions
cat ovn-kubernetes/go-controller/pkg/crd/userdefinednetwork/v1/types.go

# Questions to answer:
# - What's the spec.topology field? What values are valid?
# - What's spec.network.role (primary vs secondary)?
# - What's the difference between layer2/layer3?
# - What is localnet and how does it differ?
```

**Day 2 (Tue):** Code Investigation
```bash
# Find CUDN controller code
find ovn-kubernetes -name "*user-defined-network*" -type f

# Read controller implementation
cat ovn-kubernetes/go-controller/pkg/controller/user-defined-network/controller.go

# Focus on:
# - How layer2 CUDNs are created (OVN logical switch)
# - How layer3 CUDNs are created (OVN logical router)
# - How localnet CUDNs work (external bridge attachment)
# - Port binding logic
```

**Day 3 (Wed):** Hands-on Testing
```bash
# Use existing test clusters from vtep_unmanaged deployment

# Test 1: layer3 CUDN (already working in PoC)
kubectl apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-layer3
spec:
  network:
    subnets: ["10.200.0.0/16"]
  topology: layer3
EOF

# Verify: Check OVN logical router created
kubectl exec -n ovn-kubernetes ovnkube-node-xxx -- ovn-nbctl lr-list

# Test 2: layer2 CUDN
kubectl apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-layer2
spec:
  network:
    subnets: ["192.168.100.0/24"]
  topology: layer2
EOF

# Verify: Check OVN logical switch created
kubectl exec -n ovn-kubernetes ovnkube-node-xxx -- ovn-nbctl ls-list

# Test 3: localnet CUDN (if supported)
# Research what external bridge is needed
# Try creating and document what happens

# Document findings
vim docs/CUDN_TOPOLOGY_ANALYSIS.md
```

### Deliverable (Day 3 EOD)
- [ ] `docs/CUDN_TOPOLOGY_ANALYSIS.md` with:
  - Topology comparison table
  - OVN implementation details for each
  - Stretching feasibility assessment
  - Transport requirements (EVPN Type-2/Type-5, GRE L2/L3)
  - Recommendation: Which topologies to support in Phase 1/2/3

---

## Day 4-6: IPAM Controller Investigation 🔴

**Task:** Design broker-side IPAM for non-overlapping CIDR allocation

### Investigation Questions
1. How should IPAM allocate non-overlapping CIDRs from a user-specified pool?
2. What algorithm to split CIDRs? (e.g., /16 → /17 for 2 clusters, /18 for 4 clusters)
3. How to prevent race conditions when two agents request allocation simultaneously?
4. How to handle CIDR pool exhaustion?
5. Should we reuse CIDRs when clusters leave and rejoin?

### Daily Breakdown

**Day 4 (Thu):** Study Existing Allocators
```bash
# Review current PoC allocators
cat pkg/agent/allocator/asn_allocator.go
cat pkg/agent/allocator/vni_allocator.go
cat pkg/agent/allocator/vtep_allocator.go

# Questions:
# - How do they prevent race conditions? (optimistic locking?)
# - How do they track allocations? (in CR status?)
# - How do they handle conflicts?
# - Can we reuse the same pattern for CIDR allocation?

# Study Kubernetes IPAM implementations
# - Check how Calico does IPAM (IP pools)
# - Check how Cluster API does IPAM (InClusterIPPool)
# - What libraries exist? (go-netaddr, netip stdlib)
```

**Day 5 (Fri):** Design IPAM Algorithm
```bash
# Design CIDR splitting algorithm
# Example scenarios:

# Scenario 1: User wants 10.100.0.0/16, 2 clusters
#   Pool: 10.100.0.0/16
#   Allocation: 10.100.0.0/17 (cluster1), 10.100.128.0/17 (cluster2)

# Scenario 2: User wants 10.100.0.0/16, 4 clusters
#   Pool: 10.100.0.0/16
#   Allocation: 10.100.0.0/18 (c1), 10.100.64.0/18 (c2), 
#               10.100.128.0/18 (c3), 10.100.192.0/18 (c4)

# Scenario 3: User wants 10.100.0.0/16, 3 clusters (not power of 2)
#   Problem: Can't evenly split /16 into 3
#   Solution: Allocate /18 each (wastes 1/4 of pool), or variable sizes?

# Design considerations:
# - Fixed-size allocation (simpler, may waste space)
# - Variable-size allocation (complex, efficient)
# - Recommendation?

# Prototype algorithm in pseudocode
vim docs/IPAM_ALGORITHM_DESIGN.md
```

**Day 6 (Sat):** Design Conflict Resolution
```bash
# Research optimistic locking in Kubernetes
# - How does it work? (resourceVersion comparison)
# - Retry logic best practices
# - What if two agents request same CIDR simultaneously?

# Design conflict resolution flow:
# 1. Agent wants to join MCN "blue-network"
# 2. Agent reads MCN CR, checks status.allocations
# 3. Agent calculates next available CIDR
# 4. Agent updates MCN CR with new allocation
# 5. Kubernetes rejects if resourceVersion changed
# 6. Agent retries with new resourceVersion

# Design questions:
# - Where does allocation happen? (Broker controller or agent?)
# - If broker controller: Agent requests, broker allocates
# - If agent: Agent calculates and updates directly
# - Recommendation?

# Document design
vim docs/IPAM_CONTROLLER_DESIGN.md
```

### Deliverable (Day 6 EOD)
- [ ] `docs/IPAM_CONTROLLER_DESIGN.md` with:
  - CIDR splitting algorithm (pseudocode)
  - Conflict resolution strategy
  - Storage design (where allocations are tracked)
  - API flow (agent ↔ broker interaction)
  - Code structure outline (which packages, interfaces)

---

## Day 7-10: KubeVirt Migration Integration 🔴

**Task:** Understand how MCN can support VM live migration

### Investigation Questions
1. How does KubeVirt trigger VM migration?
2. What CRs or events can MCN agent watch to detect migration?
3. When in the migration timeline should MCN ensure network is ready?
4. Does OVN-K need changes to support VM migration, or can we integrate as-is?
5. How does VM keep same IP when moving between clusters?

### Daily Breakdown

**Day 7 (Sun):** KubeVirt Documentation Study
```bash
# Read KubeVirt migration documentation
open https://kubevirt.io/user-guide/operations/live_migration/

# Read API reference
open https://kubevirt.io/api-reference/master/definitions.html#_v1_virtualmachineinstancemigration

# Key concepts to understand:
# - VirtualMachine (VM) vs VirtualMachineInstance (VMI)
# - VirtualMachineInstanceMigration CR
# - Migration phases (scheduling, preparing, target pod, cutover)
# - How does network work during migration?

# Questions:
# - Is there multi-cluster migration support already?
# - Or is migration only intra-cluster?
# - What events are emitted during migration?
```

**Day 8 (Mon):** Hands-on KubeVirt Testing
```bash
# Install KubeVirt on test cluster
export VERSION=$(curl -s https://api.github.com/repos/kubevirt/kubevirt/releases/latest | jq -r .tag_name)
kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/${VERSION}/kubevirt-operator.yaml
kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/${VERSION}/kubevirt-cr.yaml

# Wait for KubeVirt to be ready
kubectl wait -n kubevirt kv kubevirt --for condition=Available --timeout=5m

# Create test VM
kubectl apply -f - <<EOF
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  running: true
  template:
    spec:
      domain:
        devices:
          disks:
          - name: containerdisk
            disk:
              bus: virtio
        resources:
          requests:
            memory: 512M
      volumes:
      - name: containerdisk
        containerDisk:
          image: quay.io/kubevirt/cirros-container-disk-demo
EOF

# Watch VM creation
kubectl get vmi test-vm -w

# Trigger migration (within cluster)
virtctl migrate test-vm

# Watch migration process
kubectl get virtualmachineinstancemigration -w
kubectl describe vmi test-vm

# Observe:
# - When does VMI on target cluster get created?
# - When does network switch over?
# - What labels/annotations change?
# - What events are emitted?
```

**Day 9 (Tue):** Migration Event Analysis
```bash
# Analyze migration events
kubectl get events --sort-by='.lastTimestamp' | grep test-vm

# Check VMI status during migration
kubectl get vmi test-vm -o yaml > vmi-during-migration.yaml

# Look for:
# - status.migrationState
# - status.phase changes (Running → Migrating → Running)
# - labels or annotations that indicate migration
# - When is targetNode set?

# Research multi-cluster migration
# - Is there a VirtualMachineInstanceMigration for cross-cluster?
# - Or would we need to extend KubeVirt?
# - Check KubeVirt GitHub issues for "multi-cluster migration"

# Document migration timeline
vim docs/KUBEVIRT_MIGRATION_TIMELINE.md
```

**Day 10 (Wed):** MCN Integration Design
```bash
# Design MCN integration with KubeVirt migration

# Option 1: Watch VMI CRs
# - MCN agent watches VirtualMachineInstance CRs
# - Detects when VMI.status.migrationState is set
# - Ensures CUDN is stretched to target cluster
# - Problem: How to detect cross-cluster migration vs intra-cluster?

# Option 2: Watch labels/annotations
# - User adds label: mcn.ovn.org/network=blue-network
# - MCN agent watches VMIs with this label
# - When migration detected, ensure target cluster has blue-network stretched

# Option 3: KubeVirt webhook integration
# - MCN provides admission webhook
# - KubeVirt calls webhook before migration
# - MCN ensures network is ready, approves/denies migration
# - Problem: Requires changes to KubeVirt

# Design flow diagram:
# T0: User triggers migration
# T1: MCN agent detects VMI being migrated
# T2: MCN ensures CUDN stretched to target cluster (create MCNC if needed)
# T3: OVN-K creates logical port for VM on target
# T4: Migration cutover happens
# T5: EVPN/GRE routes updated to point to target cluster
# T6: MCN cleanup on source cluster

# Open questions for KubeVirt SIG:
# - Does multi-cluster migration exist?
# - How should MCN integrate?
# - Should we contribute to KubeVirt for better integration?

# Document integration design
vim docs/KUBEVIRT_INTEGRATION.md
```

### Deliverable (Day 10 EOD)
- [ ] `docs/KUBEVIRT_INTEGRATION.md` with:
  - Migration timeline diagram
  - MCN integration points (when to act)
  - Design options (watching VMI vs labels vs webhook)
  - Recommendation: which integration approach
  - Open questions for KubeVirt SIG
  - Code structure outline for integration

---

## Summary: 10-Day Deliverables

By end of Day 10 (May 22), you will have:

### Investigation Documents
1. ✅ **CUDN_TOPOLOGY_ANALYSIS.md**
   - Topology types comparison
   - Stretching feasibility for each
   - Transport requirements
   - Phase 1/2/3 recommendations

2. ✅ **IPAM_CONTROLLER_DESIGN.md**
   - CIDR allocation algorithm
   - Conflict resolution strategy
   - API design (agent ↔ broker)
   - Code structure outline

3. ✅ **KUBEVIRT_INTEGRATION.md**
   - Migration timeline
   - MCN integration design
   - Recommended approach
   - Open questions for KubeVirt SIG

### Updated OKEP
4. ✅ **OKEP_DRAFT.md updated** with research findings
   - CUDN topology support section filled in
   - IPAM design details added
   - VM migration use case elaborated
   - Technical feasibility confirmed

### Knowledge Gained
- Deep understanding of CUDN internals
- Clear IPAM design ready for implementation
- KubeVirt integration path identified
- Confidence in technical approach

---

## How to Execute This Plan

### Daily Routine
```bash
# Morning: Read relevant documentation (1-2 hours)
# Midday: Hands-on testing/prototyping (2-3 hours)
# Afternoon: Document findings (1-2 hours)

# Example Day 1:
09:00 - 11:00  Read OVN-K CUDN docs
11:00 - 14:00  Review CUDN controller code
14:00 - 16:00  Document topology types in CUDN_TOPOLOGY_ANALYSIS.md
16:00 - 17:00  Plan Day 2 investigation
```

### Parallel Work Opportunities
- Days 4-6 (IPAM) can partially overlap with Days 1-3 (CUDN) if you want to compress timeline
- Days 7-10 (KubeVirt) requires separate focus, don't parallelize

### Checkpoints
- **Day 3 EOD:** CUDN topology analysis complete ✅
- **Day 6 EOD:** IPAM design complete ✅
- **Day 10 EOD:** KubeVirt integration design complete ✅
- **Day 11:** Update OKEP_DRAFT.md with all findings
- **Day 12:** Ready for community review!

---

## Quick Reference: Investigation Commands

### CUDN Investigation
```bash
# Check existing CUDN
kubectl get clusteruserdefinednetwork
kubectl describe cudn <name>

# Check OVN logical topology
kubectl exec -n ovn-kubernetes ovnkube-node-xxx -- ovn-nbctl show
kubectl exec -n ovn-kubernetes ovnkube-node-xxx -- ovn-nbctl lr-list
kubectl exec -n ovn-kubernetes ovnkube-node-xxx -- ovn-nbctl ls-list
```

### IPAM Investigation
```bash
# Check current allocators in PoC
cat pkg/agent/allocator/*.go

# Test CIDR splitting with Go
go run - <<EOF
package main
import ("fmt"; "net")
func main() {
    _, network, _ := net.ParseCIDR("10.100.0.0/16")
    fmt.Println(network)
    // Implement splitting logic
}
EOF
```

### KubeVirt Investigation
```bash
# Watch VM migration
kubectl get vmi -w
kubectl get virtualmachineinstancemigration -w

# Get migration details
kubectl describe virtualmachineinstancemigration <name>

# Check VM network config
kubectl get vmi <name> -o jsonpath='{.spec.networks}'
```

---

## Success Criteria

### By Day 10, you should be able to answer:

**CUDN Questions:**
- ✅ Which CUDN topologies can MCN stretch? (layer2, layer3, localnet?)
- ✅ What transport does each require? (EVPN Type-2 vs Type-5, GRE L2 vs L3)
- ✅ Which topologies should be Phase 1 vs Phase 2?

**IPAM Questions:**
- ✅ How does MCN allocate non-overlapping CIDRs from user pool?
- ✅ How are conflicts prevented when 2 agents allocate simultaneously?
- ✅ Where are allocations stored? (MCN CR status?)

**KubeVirt Questions:**
- ✅ How does MCN detect VM migration?
- ✅ When should MCN ensure network is ready on target cluster?
- ✅ Does KubeVirt need changes, or can MCN integrate read-only?
- ✅ How does VM keep same IP across clusters?

**Confidence Level:**
- ✅ OKEP technical approach is validated
- ✅ No major unknowns blocking OKEP completion
- ✅ Ready to finalize OKEP draft for community review

---

## After Day 10: Next Steps

1. **Day 11-12:** Update OKEP_DRAFT.md with research findings
2. **Day 13-14:** Add diagrams to OKEP (architecture, flows)
3. **Day 15:** Submit OKEP to OVN-K community for review

You'll be on track for community submission by May 27!

---

**Start Date:** May 12, 2026 (Today!)  
**End Date:** May 22, 2026  
**Next Review:** May 15 (after Day 3 - CUDN analysis)

Ready to start? Begin with Day 1: CUDN documentation review!
