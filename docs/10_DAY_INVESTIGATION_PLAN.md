# 13-Day Investigation Plan for OKEP
**Period:** May 12-25, 2026  
**Goal:** Complete comprehensive research to support OKEP draft

---

## Overview

This is a focused investigation plan covering the **critical research needed** for the OKEP. All tasks are research/investigation - no implementation yet.

**Total:** 4 high-priority investigation areas covering:

### 🔍 Investigation Areas

**Days 1-4:** CUDN Types & Topologies 🔴
- Topology types: layer2, layer3, localnet
- Network types: L2 vs L3 (broadcast domain vs routing)
- Role types: Primary vs Secondary networks
- Stretching feasibility and transport requirements

**Days 5-7:** IPAM Controller Design 🔴
- CIDR allocation algorithm for non-overlapping subnets
- Conflict resolution and optimistic locking
- Broker-agent API design

**Days 8-11:** KubeVirt Migration Integration 🔴
- VM live migration API and timeline
- MCN integration points
- Network continuity during migration

**Days 12-13:** Cloud Platform Support 🔴
- Multi-cloud connectivity (AWS + GCP, Azure + On-prem)
- VPN tunnels, Cloud Interconnects, security groups
- Latency, MTU, and cost considerations

---

## Day 1-4: CUDN Types & Topology Deep Dive 🔴

**Task:** Understand all CUDN types, topologies, and stretching feasibility

### Investigation Questions

**Topology Types:**
1. What are the differences between `layer2`, `layer3`, and `localnet` topologies?
2. How does OVN-K implement each internally? (OVN logical switches vs routers)
3. Which topologies can be stretched across clusters?
4. What transport requirements does each have? (EVPN Type-2 vs Type-5, GRE L2 vs L3)

**Network Types (L2 vs L3):**
5. What's the difference between L2 and L3 CUDNs?
   - L2: Same broadcast domain, ARP across clusters, MAC learning
   - L3: Routed connectivity, different broadcast domains
6. Which use cases need L2 vs L3?
7. Can we stretch both L2 and L3 CUDNs? What are the limitations?

**Role Types (Primary vs Secondary):**
8. What's `spec.network.role`? (primary vs secondary)
9. Primary network: Can it be the default pod network? Or always secondary?
10. Secondary network: Attached via Multus? How does it work?
11. Can we stretch both primary and secondary networks?
12. Are there security implications for stretching primary networks?

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

**Day 3 (Wed):** Hands-on Testing - Topology Types
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

# Test 2: layer2 CUDN (L2 switching)
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

# Deploy pods on layer2 network
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: pod-layer2-1
  annotations:
    k8s.v1.cni.cncf.io/networks: test-layer2
spec:
  containers:
  - name: test
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

# Test L2 connectivity (ARP, broadcast)
kubectl exec pod-layer2-1 -- ip neigh
kubectl exec pod-layer2-1 -- arping -c 3 <other-pod-ip>

# Test 3: localnet CUDN (if supported)
# Research what external bridge is needed
# Try creating and document what happens

# Document topology findings
vim docs/CUDN_TOPOLOGY_ANALYSIS.md
```

**Day 4 (Thu):** Testing CUDN Types - Primary vs Secondary, L2 vs L3
```bash
# Test 1: Secondary network (via Multus - default for CUDNs)
kubectl apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: secondary-net
spec:
  network:
    subnets: ["10.210.0.0/16"]
    role: secondary  # ← Secondary network
  topology: layer3
EOF

# Deploy pod with secondary network
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: pod-secondary
  annotations:
    k8s.v1.cni.cncf.io/networks: secondary-net
spec:
  containers:
  - name: test
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

# Verify pod has two interfaces (eth0=default, net1=secondary)
kubectl exec pod-secondary -- ip addr

# Test 2: Primary network (if supported)
# Research if CUDN can be primary network
# Check OVN-K documentation on primary networks
# Security implications?

# Test 3: L2 vs L3 comparison
# Create L2 CUDN and L3 CUDN side-by-side
# Compare: routing, ARP behavior, broadcast handling
# Which scenarios need L2? (VM migration, legacy apps)
# Which scenarios work with L3? (most modern apps)

# Test 4: Stretch secondary network across clusters
# Use current PoC to stretch test-layer3 (secondary, L3)
# Verify pod-to-pod connectivity
# Try stretching test-layer2 (secondary, L2)
# Check EVPN Type-2 routes in FRR

# Document findings
vim docs/CUDN_TYPES_ANALYSIS.md
```

### Deliverable (Day 4 EOD)
- [ ] `docs/CUDN_TOPOLOGY_ANALYSIS.md` with:
  - Topology comparison table (layer2/layer3/localnet)
  - OVN implementation details for each
  - Stretching feasibility assessment
  - Transport requirements (EVPN Type-2/Type-5, GRE L2/L3)
  - Recommendation: Which topologies to support in Phase 1/2/3

- [ ] `docs/CUDN_TYPES_ANALYSIS.md` with:
  - L2 vs L3 comparison (when to use each)
  - Primary vs Secondary network analysis
  - Security implications of stretching primary networks
  - Use case mapping (which CUDN type for which scenario)
  - Recommendation: Support matrix for Phase 1/2/3

---

## Day 5-7: IPAM Controller Investigation 🔴

**Task:** Design broker-side IPAM for non-overlapping CIDR allocation

### Investigation Questions
1. How should IPAM allocate non-overlapping CIDRs from a user-specified pool?
2. What algorithm to split CIDRs? (e.g., /16 → /17 for 2 clusters, /18 for 4 clusters)
3. How to prevent race conditions when two agents request allocation simultaneously?
4. How to handle CIDR pool exhaustion?
5. Should we reuse CIDRs when clusters leave and rejoin?

### Daily Breakdown

**Day 5 (Fri):** Study Existing Allocators
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

**Day 6 (Sat):** Design IPAM Algorithm
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

**Day 7 (Sun):** Design Conflict Resolution
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

### Deliverable (Day 7 EOD)
- [ ] `docs/IPAM_CONTROLLER_DESIGN.md` with:
  - CIDR splitting algorithm (pseudocode)
  - Conflict resolution strategy
  - Storage design (where allocations are tracked)
  - API flow (agent ↔ broker interaction)
  - Code structure outline (which packages, interfaces)

---

## Day 8-11: KubeVirt Migration Integration 🔴

**Task:** Understand how MCN can support VM live migration

### Investigation Questions
1. How does KubeVirt trigger VM migration?
2. What CRs or events can MCN agent watch to detect migration?
3. When in the migration timeline should MCN ensure network is ready?
4. Does OVN-K need changes to support VM migration, or can we integrate as-is?
5. How does VM keep same IP when moving between clusters?

### Daily Breakdown

**Day 8 (Mon):** KubeVirt Documentation Study
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

**Day 9 (Tue):** Hands-on KubeVirt Testing
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

**Day 10 (Wed):** Migration Event Analysis
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

**Day 11 (Thu):** MCN Integration Design
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

### Deliverable (Day 11 EOD)
- [ ] `docs/KUBEVIRT_INTEGRATION.md` with:
  - Migration timeline diagram
  - MCN integration points (when to act)
  - Design options (watching VMI vs labels vs webhook)
  - Recommendation: which integration approach
  - Open questions for KubeVirt SIG
  - Code structure outline for integration

---

## Day 12-13: Cloud Platform Support Investigation 🔴

**Task:** Investigate stretching CUDNs across different cloud platforms (AWS + GCP, Azure + On-prem, etc.)

### Investigation Questions

**Multi-Cloud Networking:**
1. Can MCN stretch CUDNs when clusters run on different cloud providers?
   - Example: Cluster1 on AWS, Cluster2 on GCP
   - Example: Cluster1 on Azure, Cluster2 on on-premises datacenter
2. What networking challenges exist for cross-cloud CUDN stretching?
3. How do cloud providers handle inter-cloud connectivity?

**Connectivity Options:**
4. **VPN Tunnels:** AWS VPN ↔ GCP VPN
   - Can BGP peer over VPN tunnels?
   - Latency impact on EVPN convergence?
   - MTU considerations (VPN overhead + VXLAN/GRE overhead)
5. **Cloud Interconnects:** AWS Direct Connect ↔ GCP Cloud Interconnect
   - Private connectivity between clouds
   - Bandwidth and latency characteristics
   - Cost implications
6. **Public Internet:** Nodes with public IPs
   - Security concerns (BGP over internet)
   - Need for encryption (IPsec overlay)
   - Firewall rules and security groups

**Cloud-Specific Challenges:**
7. **AWS-specific:**
   - Security groups: Allow BGP (TCP 179), VXLAN (UDP 4789), GRE
   - VPC peering limitations
   - Transit Gateway for multi-VPC
8. **GCP-specific:**
   - Firewall rules: Allow BGP, VXLAN, GRE
   - VPC network peering
   - Cloud Router for BGP
9. **Azure-specific:**
   - Network Security Groups (NSGs)
   - VNet peering
   - Azure ExpressRoute

**IP Addressing:**
10. Do cloud providers allow custom VTEP IPs (100.0.0.0/8)?
11. Overlapping VPC CIDRs: If AWS VPC uses 10.0.0.0/16 and GCP VPC uses 10.0.0.0/16?
12. Public vs private IPs for BGP peering endpoints?

### Daily Breakdown

**Day 12 (Fri):** Cloud Networking Research
```bash
# Research AWS networking
# - How to establish VPN to GCP?
# - AWS Transit Gateway for multi-cluster
# - Security group rules for BGP/VXLAN/GRE
# - Can KIND clusters on AWS EC2 communicate with GCP GKE?

# Research GCP networking
# - Cloud VPN to AWS
# - Cloud Interconnect options
# - Firewall rules for BGP/VXLAN/GRE
# - GKE cluster networking model

# Research Azure networking
# - VPN Gateway to AWS/GCP
# - ExpressRoute for private connectivity
# - NSG rules

# Research multi-cloud connectivity patterns
# - Aviatrix (multi-cloud networking)
# - HashiCorp Consul Connect
# - Service mesh solutions (Istio multi-cluster)
# - How do they handle cross-cloud connectivity?

# Document findings
vim docs/CLOUD_PLATFORM_NETWORKING.md
```

**Day 13 (Sat):** Cloud Platform Testing Plan
```bash
# Design test scenarios for cloud platform support

# Scenario 1: AWS EKS + GCP GKE
# - Deploy EKS cluster in us-east-1
# - Deploy GKE cluster in us-central1
# - Establish VPN tunnel between AWS VPC and GCP VPC
# - Configure security groups / firewall rules
# - Test MCN CUDN stretching over VPN

# Scenario 2: On-prem + AWS
# - On-prem cluster (existing KIND clusters)
# - AWS EKS cluster
# - VPN from on-prem to AWS VPC
# - Test MCN over VPN tunnel

# Scenario 3: Public internet peering
# - Clusters with public IPs
# - BGP peering over internet (with IPsec)
# - Security considerations

# Challenges to document:
# - MTU issues (VPN overhead + VXLAN/GRE encap)
# - Latency impact (cross-region, cross-cloud)
# - Firewall rule management
# - Cost of inter-cloud traffic
# - Security best practices

# Design cloud platform support matrix
vim docs/CLOUD_PLATFORM_SUPPORT_MATRIX.md

# Example matrix:
# | Cloud Combo | Connectivity | MCN Transport | Feasibility | Notes |
# |-------------|--------------|---------------|-------------|-------|
# | AWS + GCP   | VPN          | EVPN          | ✅ Yes      | VPN overhead |
# | AWS + GCP   | Interconnect | EVPN          | ✅ Yes      | Low latency |
# | AWS + Azure | VPN          | EVPN          | ✅ Yes      | Cross-cloud VPN |
# | On-prem + AWS | VPN        | GRE           | ✅ Yes      | HW offload on-prem |
```

### Cloud Platform Investigation Sub-Tasks

**Sub-task 1: Security Group / Firewall Rules**
```bash
# AWS Security Group rules needed for MCN
# Ingress rules:
# - TCP 179 (BGP)
# - UDP 4789 (VXLAN) or Protocol 47 (GRE)
# - Custom ICMP for debugging

# GCP Firewall rules
# - Allow BGP from remote cluster IPs
# - Allow VXLAN/GRE from remote cluster IPs

# Document required firewall rules for each cloud
```

**Sub-task 2: VPN Tunnel Configuration**
```bash
# Research VPN setup between clouds
# - AWS VPN Gateway
# - GCP Cloud VPN
# - Site-to-site VPN configuration
# - BGP over VPN (dynamic routing)

# MTU calculations:
# - Standard MTU: 1500
# - VPN overhead: ~50-100 bytes (IPsec)
# - VXLAN overhead: 50 bytes
# - GRE overhead: 24 bytes
# - Result: May need to reduce pod MTU to 1350-1400
```

**Sub-task 3: Latency and Performance**
```bash
# Research expected latency for cross-cloud
# - AWS us-east-1 ↔ GCP us-central1: ~20-30ms
# - AWS us-east-1 ↔ GCP europe-west1: ~80-100ms
# - On-prem ↔ AWS: varies (10-50ms typical)

# How does latency impact MCN?
# - BGP convergence time
# - EVPN route propagation
# - Application performance (pod-to-pod latency)

# Recommendations for latency-sensitive workloads?
```

**Sub-task 4: Cost Considerations**
```bash
# Research inter-cloud data transfer costs
# - AWS → GCP: $0.01-0.09 per GB (depends on region)
# - GCP → AWS: Similar pricing
# - VPN tunnel costs: ~$0.05/hour per tunnel
# - Direct Connect / Interconnect: Higher upfront, lower per-GB cost

# Document cost implications for MCN users
# - Stretching CUDN across clouds = inter-cloud data transfer
# - Recommendations: Use for control plane, keep data plane local?
```

### Deliverable (Day 13 EOD)
- [ ] `docs/CLOUD_PLATFORM_NETWORKING.md` with:
  - Multi-cloud connectivity options (VPN, Interconnect, Public)
  - Security group / firewall rule requirements for each cloud
  - VPN tunnel setup guide
  - MTU considerations
  - Latency expectations

- [ ] `docs/CLOUD_PLATFORM_SUPPORT_MATRIX.md` with:
  - Cloud combination support matrix (AWS+GCP, AWS+Azure, etc.)
  - Recommended connectivity method for each combo
  - Known limitations and challenges
  - Cost considerations
  - Phase 1/2/3 support roadmap

- [ ] Test plan for multi-cloud validation (for future Phase 2 testing)

---

## Summary: 13-Day Deliverables

By end of Day 13 (May 25), you will have:

### Investigation Documents
1. ✅ **CUDN_TOPOLOGY_ANALYSIS.md**
   - Topology types comparison (layer2/layer3/localnet)
   - Stretching feasibility for each
   - Transport requirements (EVPN Type-2/Type-5, GRE L2/L3)
   - Phase 1/2/3 recommendations

2. ✅ **CUDN_TYPES_ANALYSIS.md**
   - L2 vs L3 networking comparison
   - Primary vs Secondary network analysis
   - Use case mapping (which type for which scenario)
   - Security implications
   - Support matrix for Phase 1/2/3

3. ✅ **IPAM_CONTROLLER_DESIGN.md**
   - CIDR allocation algorithm (pseudocode)
   - Conflict resolution strategy
   - API design (agent ↔ broker)
   - Code structure outline

4. ✅ **KUBEVIRT_INTEGRATION.md**
   - Migration timeline diagram
   - MCN integration design
   - Recommended approach
   - Open questions for KubeVirt SIG

5. ✅ **CLOUD_PLATFORM_NETWORKING.md**
   - Multi-cloud connectivity options
   - Security group / firewall rules
   - VPN tunnel setup guide
   - MTU and latency considerations

6. ✅ **CLOUD_PLATFORM_SUPPORT_MATRIX.md**
   - Cloud combination support matrix
   - Recommended connectivity per combo
   - Known limitations and challenges
   - Cost considerations

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
- **Day 4 EOD:** CUDN types & topology analysis complete ✅
- **Day 7 EOD:** IPAM design complete ✅
- **Day 11 EOD:** KubeVirt integration design complete ✅
- **Day 13 EOD:** Cloud platform support analysis complete ✅
- **Day 14-15:** Update OKEP_DRAFT.md with all findings
- **Day 16:** Ready for community review!

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

### By Day 13, you should be able to answer:

**CUDN Topology Questions:**
- ✅ Which CUDN topologies can MCN stretch? (layer2, layer3, localnet?)
- ✅ What transport does each require? (EVPN Type-2 vs Type-5, GRE L2 vs L3)
- ✅ Which topologies should be Phase 1 vs Phase 2?

**CUDN Type Questions:**
- ✅ What's the difference between L2 and L3 CUDNs? When to use each?
- ✅ What's the difference between Primary and Secondary networks?
- ✅ Can MCN stretch both Primary and Secondary networks?
- ✅ Are there security implications for stretching Primary networks?

**IPAM Questions:**
- ✅ How does MCN allocate non-overlapping CIDRs from user pool?
- ✅ How are conflicts prevented when 2 agents allocate simultaneously?
- ✅ Where are allocations stored? (MCN CR status?)

**KubeVirt Questions:**
- ✅ How does MCN detect VM migration?
- ✅ When should MCN ensure network is ready on target cluster?
- ✅ Does KubeVirt need changes, or can MCN integrate read-only?
- ✅ How does VM keep same IP across clusters?

**Cloud Platform Questions:**
- ✅ Can MCN stretch CUDNs across different cloud providers? (AWS + GCP, Azure + On-prem)
- ✅ What connectivity is needed? (VPN, Interconnect, Public Internet)
- ✅ What security group / firewall rules are required?
- ✅ What are the latency and cost implications?
- ✅ Which cloud combinations should be supported in Phase 1/2/3?

**Confidence Level:**
- ✅ OKEP technical approach is validated
- ✅ No major unknowns blocking OKEP completion
- ✅ Ready to finalize OKEP draft for community review

---

## After Day 13: Next Steps

1. **Day 14-15:** Update OKEP_DRAFT.md with research findings
   - Add CUDN types analysis to Design Details section
   - Add cloud platform support section
   - Update use cases with L2/L3 and multi-cloud scenarios
2. **Day 16-17:** Add diagrams to OKEP (architecture, flows, cloud connectivity)
3. **Day 18:** Submit OKEP to OVN-K community for review

You'll be on track for community submission by May 30!

---

**Start Date:** May 12, 2026 (Today!)  
**End Date:** May 25, 2026  
**Next Review:** May 16 (after Day 4 - CUDN types & topology analysis)

Ready to start? Begin with Day 1: CUDN documentation review!
