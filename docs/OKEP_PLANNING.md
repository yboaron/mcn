# OKEP Planning: Multi-Cluster Networking for OVN-Kubernetes

**Status:** Planning Phase  
**Target:** Draft OKEP for community review  
**Repo:** New repo under ovn-kubernetes organization  
**Meeting Feedback:** 2026-05-11 OVN-K Community Meeting

---

## Meeting Summary

### Positive Feedback
- ✅ Community wants to adopt multi-cluster as separate layer on top of OVN-K
- ✅ Agreement to create new repo under ovn-kubernetes organization
- ✅ Cloud platforms interested
- ✅ NVIDIA interested in multi-cluster connectivity
- ✅ Strong interest in VM Live Migration use case

### Key Concerns Raised

**A. Transport Flexibility (Beyond EVPN)**
- Current PoC uses EVPN only
- **Request:** Support GRE for hardware offload
- **Request:** Support other transport mechanisms
- **Question:** What transport types does CUDN support? How do we stretch each?

**B. IPAM Across Clusters (Conflicting CIDRs)**
- **Problem:** Blue CUDN on cluster1 with CIDR=10.100.0.0/16
- **Problem:** Blue CUDN on cluster2 with same CIDR=10.100.0.0/16
- **Solution needed:** Broker-layer IPAM to ensure non-overlapping CIDRs
- **Goal:** Same CUDN name across clusters should have coordinated, non-overlapping CIDRs

**C. CUDN Type Coverage**
- **Primary vs Secondary** networks
- **L2 vs L3** CUDNs
- **Different topology modes** (local, layer2, layer3, localnet)
- **Question:** Need to support all variants or subset?

**D. VM Live Migration**
- **Use case:** Migrate VMs across clusters while maintaining network connectivity
- **Integration:** KubeVirt + OVN-K + Multi-cluster layer
- **Requirement:** Network continuity during migration

### Next Steps (Community Request)
1. **OKEP** - Enhancement proposal capturing high-level use cases
2. **New repo proposal** - Under ovn-kubernetes organization
3. **Address all concerns** raised in meeting

---

## OKEP Structure (Draft Outline)

### 1. Title & Metadata
- **KEP Number:** TBD (assigned by OVN-K maintainers)
- **Title:** Multi-Cluster Networking for OVN-Kubernetes
- **Authors:** Yossi Boaron, [contributors TBD]
- **Status:** Provisional
- **Creation Date:** 2026-05-12
- **Target OVN-K Version:** v24.09+ (requires EVPN, VTEP, RouteAdvertisement support)

### 2. Summary (1 paragraph)
Extend OVN-Kubernetes User Defined Networks (UDNs) across multiple Kubernetes clusters through an external orchestration layer that manages network stretching, BGP peering, IPAM coordination, and transport configuration.

### 3. Motivation

#### Goals
- Enable **CUDN stretching** across multiple clusters with minimal user intervention
- Support **multiple transport mechanisms** (EVPN, GRE, others) based on use case
- Provide **cross-cluster IPAM** to prevent CIDR conflicts
- Enable **VM Live Migration** across clusters
- Support all CUDN types: Primary/Secondary, L2/L3, all topology modes
- Maintain **OVN-K native APIs** - no forking, use existing CRDs where possible
- **Non-invasive** - work with upstream OVN-K, external orchestration layer

#### Non-Goals (Initial Phase)
- Stretching default network across clusters (security/isolation concerns)
- Cross-cluster Service discovery (Submariner or other projects handle this)
- Multi-cluster policy federation (future work)
- Cross-cluster DNS (delegate to Submariner/CoreDNS)

#### Use Cases

**UC1: Disaster Recovery / Geographic Redundancy**
- **Actor:** Platform operator
- **Goal:** Stretch application networks across geographically distributed clusters
- **Benefit:** Workload can fail over to another cluster while maintaining network identity
- **Example:** Blue CUDN spans US-East and US-West clusters, pods can migrate on failure

**UC2: Workload Migration / Bursting**
- **Actor:** Application developer
- **Goal:** Move workloads between clusters without changing network config
- **Benefit:** Scale to second cluster during peak load, migrate back during off-peak
- **Example:** Batch processing workload migrates from on-prem to cloud during demand spike

**UC3: VM Live Migration Across Clusters**
- **Actor:** Virtualization platform operator (KubeVirt)
- **Goal:** Live migrate VMs between clusters while maintaining network connectivity
- **Benefit:** Cluster maintenance, load balancing, cost optimization
- **Requirements:**
  - Network continuity during migration (same IP, same L2 segment if needed)
  - Minimal downtime (sub-second)
  - Integration with KubeVirt migration API
- **Example:** Migrate GPU-heavy VM from cluster1 to cluster2 with newer GPUs

**UC4: Multi-Cluster Service Mesh**
- **Actor:** Service mesh operator
- **Goal:** Extend mesh network (e.g., Istio sidecar network) across clusters
- **Benefit:** Unified mesh with pod-to-pod connectivity across boundaries
- **Example:** Green CUDN for Istio control plane spans 3 clusters

**UC5: Edge / Hub-and-Spoke Deployments**
- **Actor:** Edge platform operator
- **Goal:** Connect edge clusters to central hub via stretched networks
- **Benefit:** Centralized management, data aggregation, consistent networking
- **Example:** 10 edge sites with Orange CUDN connected to hub cluster

**UC6: Hardware Offload Optimization**
- **Actor:** Cloud provider with SmartNIC hardware
- **Goal:** Use GRE or other HW-offloadable transport instead of EVPN
- **Benefit:** Higher throughput, lower CPU usage, leverage hardware capabilities
- **Example:** Stretch CUDN using GRE tunnels offloaded to Mellanox BlueField NICs

**UC7: Non-Overlapping CIDR Coordination**
- **Actor:** Multi-cluster platform operator
- **Goal:** Create same CUDN name in multiple clusters with coordinated CIDRs
- **Problem:** Without coordination, both clusters might allocate 10.100.0.0/16 to "blue" CUDN
- **Solution:** Broker-managed IPAM allocates non-overlapping blocks (cluster1: 10.100.0.0/17, cluster2: 10.100.128.0/17)
- **Benefit:** No conflicts, seamless stretching, automated CIDR management

### 4. Proposal

#### Architecture Overview

**Broker-Agent Pattern (Submariner-inspired):**
- **Broker:** Pure storage layer (Kubernetes cluster with CRDs), no controllers
- **Agent:** Runs on each managed cluster, does all orchestration
- **Sync:** Bidirectional sync via Submariner Admiral

**Components:**
1. **Broker Cluster:** Stores multi-cluster state (Cluster CRs, MultiClusterNetwork CRs)
2. **MCN Agent:** Deployed on each cluster, manages local network configuration
3. **CRDs:** 
   - `Cluster` (broker) - cluster registration with endpoints, ASN, VTEP CIDR
   - `MultiClusterNetwork` (broker) - network registry with VNI, RT, participating clusters
   - `MultiClusterNetworkConnect` (local) - intent to join CUDN to MCN
4. **OVN-K Integration:** Agent creates VTEP, CUDN patches, RouteAdvertisements, FRRConfiguration

#### API Design

**User Workflow:**
```yaml
# 1. User creates CUDN locally (standard OVN-K)
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
spec:
  network:
    subnets: ["10.100.0.0/16"]  # Agent will coordinate this via broker IPAM
  topology: layer3

# 2. User stretches CUDN across clusters (MCN-specific)
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  multiClusterNetwork: blue-network  # MCN name (same across clusters)
  localNetwork: blue-network         # Local CUDN name
  transport: evpn                    # or 'gre', 'geneve', etc.
```

**Broker-Managed IPAM:**
- User specifies desired CIDR in CUDN spec (e.g., 10.100.0.0/16)
- Agent sends request to broker with CIDR preference
- Broker IPAM controller allocates non-overlapping subnet from pool
- Returns allocated CIDR to agent (e.g., 10.100.0.0/17 for cluster1, 10.100.128.0/17 for cluster2)
- Agent patches CUDN with allocated CIDR
- Result: Same CUDN name, non-overlapping CIDRs, seamless stretching

#### Transport Abstraction

**Challenge:** Different transports have different requirements:
- **EVPN:** Requires BGP peering, VNI allocation, route targets, FRR-K8s
- **GRE:** Requires GRE tunnel config, endpoint IP discovery, no BGP
- **GENEVE:** Similar to VXLAN but different encap, OVN native
- **IPsec overlay:** Encrypted tunnels, certificate management

**Proposal:** Transport plugin interface
```go
type TransportPlugin interface {
    // Configure transport for this cluster
    ConfigureLocal(ctx context.Context, mcn *MultiClusterNetwork, endpoints []Endpoint) error
    
    // Cleanup transport configuration
    Cleanup(ctx context.Context, mcn *MultiClusterNetwork) error
    
    // Get transport-specific status
    Status(ctx context.Context, mcn *MultiClusterNetwork) (*TransportStatus, error)
}

// Implementations:
// - EVPNTransport: Current PoC implementation (BGP, FRR-K8s, VNI, RT)
// - GRETransport: GRE tunnel manager (OVN tunnel config, endpoint discovery)
// - GeneveTransport: GENEVE encap (OVN native, similar to current local gateway)
```

**User specifies transport in MCNC:**
```yaml
spec:
  transport: gre  # or evpn, geneve, ipsec
  transportConfig:
    # GRE-specific options
    ttl: 64
    key: auto
    offload: true  # Use hardware offload if available
```

#### CUDN Type Support

**Primary vs Secondary Networks:**
- **Primary:** Default network for pods (spec.network.role: "primary")
- **Secondary:** Additional networks via Multus (spec.network.role: "secondary")
- **OKEP Scope:** Support both, most common use case is Secondary

**L2 vs L3:**
- **L2 (topology: layer2):** Pods in same L2 domain, ARP across clusters
- **L3 (topology: layer3):** Pods in same L3 network, routed connectivity
- **OKEP Scope:** Support both (PoC already handles L3, need to validate L2)

**Topology Modes:**
- `localnet`: Local network bridge (how to stretch? Research needed)
- `layer2`: L2 switching (EVPN Type-2 routes, GRE L2 tunnels)
- `layer3`: L3 routing (EVPN Type-5 routes, GRE L3 tunnels)
- **OKEP Scope:** layer2 and layer3 first, localnet TBD

#### VM Live Migration Support

**Requirements:**
1. **Network continuity:** VM keeps same IP during migration
2. **L2 connectivity:** If VM relies on L2 adjacency (ARP, broadcast)
3. **Minimal downtime:** Sub-second network disruption
4. **KubeVirt integration:** Detect migration events, trigger network updates

**Proposed Flow:**
```
1. KubeVirt initiates migration (VM on cluster1 → cluster2)
   ↓
2. MCN Agent on cluster2 detects incoming VM (via KubeVirt event/label)
   ↓
3. Agent ensures CUDN is stretched to cluster2 (create MCNC if needed)
   ↓
4. OVN-K creates logical port for VM on cluster2 with same IP
   ↓
5. EVPN/GRE propagates route update (IP now reachable via cluster2 VTEP)
   ↓
6. KubeVirt completes migration
   ↓
7. Agent on cluster1 cleans up old VM network config
```

**Open Questions:**
- How does MCN agent detect migration? (Watch VirtualMachineInstance CRD?)
- Does OVN-K need changes to support VM migration? (Coordinate with KubeVirt SIG)
- Can we reuse existing KubeVirt multi-cluster migration? (Research needed)

### 5. Design Details

#### IPAM Across Clusters

**Problem:**
- Cluster1 admin creates "blue" CUDN with 10.100.0.0/16
- Cluster2 admin creates "blue" CUDN with 10.100.0.0/16
- User tries to stretch "blue" CUDN → CIDR conflict!

**Solution 1: Broker-Managed IPAM (Recommended)**
```
1. User creates CUDN with desired CIDR (e.g., 10.100.0.0/16)
2. User creates MCNC to stretch CUDN
3. Agent detects MCNC, sends IPAM request to broker:
   - MCN name: "blue"
   - Desired CIDR: 10.100.0.0/16
   - Cluster ID: cluster1
4. Broker IPAM controller:
   - Checks if MCN "blue" exists
   - If new: Allocate pool from desired CIDR (10.100.0.0/16)
   - If exists: Check if pool matches (10.100.0.0/16 ✓)
   - Allocate non-overlapping subnet for cluster1: 10.100.0.0/17
5. Agent receives allocated CIDR: 10.100.0.0/17
6. Agent patches CUDN spec.network.subnets to use allocated CIDR
7. Repeat for cluster2: receives 10.100.128.0/17
8. Result: Same MCN name, non-overlapping CIDRs, coordinated allocation
```

**Solution 2: Pre-Allocated CIDR Blocks**
- Operator pre-configures CIDR blocks per cluster (cluster1: 10.100.0.0/17, cluster2: 10.100.128.0/17)
- User must create CUDN with cluster-specific CIDR
- No dynamic IPAM, manual coordination required
- **Downside:** Error-prone, manual work, doesn't scale

**Recommendation:** Solution 1 (Broker-Managed IPAM)

**Implementation:**
- New controller on broker: `IPAMController`
- Watches `MultiClusterNetwork` CRs
- Allocates subnets using bitmap allocator (similar to ASN/VNI allocators)
- Stores allocation in MCN status: `status.allocations[clusterID].cidr`

#### Transport Plugin Architecture

**Interface:**
```go
// pkg/agent/transport/interface.go
type Transport interface {
    // Name returns transport identifier (e.g., "evpn", "gre")
    Name() string
    
    // Configure sets up transport for given MCN and remote endpoints
    Configure(ctx context.Context, config *TransportConfig) error
    
    // Cleanup removes transport configuration
    Cleanup(ctx context.Context, mcn *MultiClusterNetwork) error
    
    // Status returns current transport state
    Status(ctx context.Context) (*TransportStatus, error)
}

type TransportConfig struct {
    MCN             *MultiClusterNetwork
    LocalCUDN       *ovnv1.ClusterUserDefinedNetwork
    RemoteEndpoints []Endpoint
    LocalASN        int32  // Only for BGP-based transports
    VNI             int32  // Only for VXLAN/GENEVE
    TransportParams map[string]string  // Transport-specific config
}
```

**Implementations:**

**1. EVPN Transport (Current PoC):**
```go
// pkg/agent/transport/evpn/evpn.go
type EVPNTransport struct {
    bgpConfigurator *bgp.BGPConfigurator
    vtepManager     *vtep.VtepManager
}

func (e *EVPNTransport) Configure(ctx context.Context, config *TransportConfig) error {
    // 1. Create VTEP for remote clusters
    // 2. Configure BGP peering (FRRConfiguration)
    // 3. Create RouteAdvertisement with VNI/RT
    // 4. Patch CUDN with EVPN config
}
```

**2. GRE Transport (New):**
```go
// pkg/agent/transport/gre/gre.go
type GRETransport struct {
    ovnClient dynamic.Interface
}

func (g *GRETransport) Configure(ctx context.Context, config *TransportConfig) error {
    // 1. Discover remote endpoint IPs from broker
    // 2. Create OVN tunnel config (ovn-nbctl set NB_Global . tunnel_encaps=gre)
    // 3. Configure GRE tunnel endpoints
    // 4. Set up routing for remote CUDN subnets via GRE tunnels
    // 5. Enable hardware offload if supported (via ethtool/sysctl)
}
```

**Transport Selection Logic:**
```yaml
# User specifies in MCNC
spec:
  transport: gre
  
# Agent selects transport plugin
transport := transport.NewTransport(mcnc.Spec.Transport)
transport.Configure(ctx, config)
```

#### CUDN Topology Support Matrix

| Topology | L2/L3 | EVPN Support | GRE Support | Stretch Complexity |
|----------|-------|--------------|-------------|-------------------|
| layer3   | L3    | ✅ Type-5 routes | ✅ L3 GRE | Low (PoC working) |
| layer2   | L2    | ✅ Type-2 routes | ✅ L2 GRE | Medium (need to test) |
| localnet | L2    | ❓ Research needed | ❓ Research needed | High (external bridge) |

**Phase 1 Scope:** layer3 (already working in PoC)  
**Phase 2 Scope:** layer2 (validate EVPN Type-2, implement GRE L2)  
**Phase 3 Scope:** localnet (research feasibility)

### 6. Risks and Mitigations

#### Risk 1: OVN-K VTEP Managed Mode Incomplete
- **Risk:** Current OVN-K doesn't fully implement managed VTEP (doesn't assign IPs to loopback)
- **Impact:** Can't use automatic VTEP IP allocation
- **Mitigation:** Use unmanaged VTEP mode (manual IP assignment) until OVN-K completes implementation
- **Long-term:** Contribute managed VTEP implementation to OVN-K upstream

#### Risk 2: IPAM Complexity
- **Risk:** Broker-managed IPAM adds complexity, potential for conflicts
- **Impact:** CIDRs might still conflict if IPAM has bugs
- **Mitigation:** 
  - Comprehensive testing with conflict scenarios
  - Optimistic locking for IPAM allocations
  - Clear error messages when conflicts detected
  - Allow manual CIDR override for advanced users

#### Risk 3: Transport Diversity
- **Risk:** Supporting multiple transports increases maintenance burden
- **Impact:** More code to maintain, test, debug
- **Mitigation:**
  - Start with EVPN only (Phase 1)
  - Add GRE in Phase 2 based on demand
  - Use plugin architecture to isolate transport-specific code
  - Comprehensive transport integration tests

#### Risk 4: VM Migration Coordination
- **Risk:** KubeVirt integration might require changes to both projects
- **Impact:** Slower adoption, dependency on KubeVirt release cycle
- **Mitigation:**
  - Engage with KubeVirt SIG early
  - Design agent to detect migration without KubeVirt changes if possible
  - Document integration requirements clearly

#### Risk 5: Scale and Performance
- **Risk:** Full mesh BGP doesn't scale to 100+ clusters
- **Impact:** BGP session explosion, slow convergence
- **Mitigation:**
  - Route reflector topology (Phase 2)
  - Hierarchical route aggregation
  - Cluster grouping for large deployments
  - Document scale limits clearly

### 7. Test Plan

#### Unit Tests
- IPAM allocator (conflict detection, allocation, deallocation)
- Transport plugin interface (mock plugins)
- CUDN type compatibility checks

#### Integration Tests
- Two-cluster CUDN stretching (EVPN)
- Two-cluster CUDN stretching (GRE)
- CIDR conflict resolution via IPAM
- VM migration simulation (KubeVirt + MCN)
- Layer2 CUDN stretching
- Layer3 CUDN stretching

#### E2E Tests
- Deploy MCN on 3-cluster setup
- Stretch multiple CUDNs simultaneously
- Pod-to-pod connectivity across clusters
- Network isolation (different CUDNs can't communicate)
- Cluster join/leave (add 4th cluster, remove 1st cluster)
- Transport switching (migrate from EVPN to GRE)

#### Scale Tests
- 10 clusters, full mesh BGP
- 50 CUDNs stretched across clusters
- 1000 pods per CUDN
- BGP convergence time measurement
- IPAM allocation performance (1000 allocations)

### 8. Graduation Criteria

#### Alpha (Phase 1)
- ✅ OKEP approved by community
- ✅ New repo created under ovn-kubernetes org
- ✅ Basic CUDN stretching with EVPN (layer3)
- ✅ Broker-managed IPAM (basic)
- ✅ Two-cluster E2E tests passing
- ⚠️ Known limitations documented (VTEP managed mode, scale limits)

#### Beta (Phase 2)
- ✅ GRE transport support
- ✅ Layer2 CUDN stretching
- ✅ VM migration support (KubeVirt integration)
- ✅ Route reflector topology
- ✅ Multi-cluster (3+) E2E tests passing
- ✅ Scale testing completed (10 clusters, 50 CUDNs)
- ✅ Production deployment guides

#### GA (Phase 3)
- ✅ All transport types stable
- ✅ Advanced IPAM features (CIDR migration, pool management)
- ✅ Observability (metrics, tracing, dashboards)
- ✅ Security review completed
- ✅ Multiple production deployments
- ✅ Performance benchmarks published

### 9. Implementation Phases

#### Phase 1: Core Functionality (3-4 months)
**Goal:** Get basic CUDN stretching working with IPAM

**Deliverables:**
1. OKEP approved and published
2. New repo created: `ovn-kubernetes/ovn-mcn` (or similar name)
3. Broker-managed IPAM implementation
4. EVPN transport (refactor current PoC)
5. Layer3 CUDN stretching working
6. Two-cluster E2E tests
7. Documentation: Quick start, architecture guide

**Milestones:**
- Week 1-2: OKEP review and approval
- Week 3-4: Repo setup, CI/CD, basic structure
- Week 5-8: IPAM implementation and tests
- Week 9-12: EVPN transport refactoring
- Week 13-16: E2E tests, documentation, alpha release

#### Phase 2: Transport Diversity & VM Migration (3-4 months)
**Goal:** Support GRE, layer2, and VM migration

**Deliverables:**
1. Transport plugin architecture
2. GRE transport implementation
3. Layer2 CUDN stretching (EVPN Type-2, GRE L2)
4. KubeVirt integration for VM migration
5. Route reflector topology support
6. Multi-cluster (3+) E2E tests

**Milestones:**
- Week 1-4: Transport plugin interface
- Week 5-8: GRE transport implementation
- Week 9-10: Layer2 stretching validation
- Week 11-14: KubeVirt integration
- Week 15-16: Route reflector, beta release

#### Phase 3: Production Readiness (2-3 months)
**Goal:** Make it production-ready

**Deliverables:**
1. Advanced IPAM (CIDR migration, pool management)
2. Observability (Prometheus metrics, OpenTelemetry tracing)
3. Security hardening (RBAC review, network policies)
4. Scale testing (10+ clusters, 50+ CUDNs)
5. Performance tuning
6. Production deployment guides
7. Troubleshooting playbooks

**Milestones:**
- Week 1-4: Observability and monitoring
- Week 5-6: Security review and hardening
- Week 7-10: Scale and performance testing
- Week 11-12: Documentation and GA release

### 10. Alternatives Considered

#### Alternative 1: In-tree OVN-K Controllers
**Approach:** Build multi-cluster logic into OVN-K controllers themselves
- **Pros:** Tighter integration, single binary
- **Cons:** Invasive changes, harder to maintain, slower iteration
- **Decision:** Rejected - community prefers external layer

#### Alternative 2: Use Submariner Directly
**Approach:** Extend Submariner to support CUDN stretching
- **Pros:** Reuse existing multi-cluster project
- **Cons:** Different architecture (gateway model vs full mesh), different focus (services vs networks)
- **Decision:** Rejected - complementary projects, could integrate later

#### Alternative 3: No IPAM Coordination
**Approach:** Require users to manually ensure non-overlapping CIDRs
- **Pros:** Simpler implementation
- **Cons:** Error-prone, doesn't scale, bad UX
- **Decision:** Rejected - IPAM coordination is critical for usability

#### Alternative 4: Single Transport (EVPN Only)
**Approach:** Support only EVPN, no GRE or other transports
- **Pros:** Simpler code, less maintenance
- **Cons:** Doesn't address hardware offload use case, less flexible
- **Decision:** Hybrid - Start with EVPN (Phase 1), add GRE (Phase 2)

---

## Research Tasks

### Task 1: CUDN Transport Types Deep Dive
**Goal:** Understand all CUDN transport/topology types and how to stretch each
**Questions:**
- What's the difference between localnet, layer2, layer3 topologies?
- How does OVN-K implement each? (OVN logical switches, ports, routes?)
- Which can be stretched? Which cannot? Why?
- What's needed for each transport (EVPN Type-2 vs Type-5, GRE L2 vs L3)?

**Method:**
- Read OVN-K CUDN documentation and code
- Test each topology type locally
- Document findings in `docs/CUDN_TOPOLOGY_ANALYSIS.md`

### Task 2: GRE Hardware Offload Research
**Goal:** Understand GRE hardware offload requirements and capabilities
**Questions:**
- Which NICs support GRE offload? (Mellanox BlueField, Intel E810?)
- How to enable offload? (ethtool, kernel params, NIC firmware?)
- Performance comparison: EVPN vs GRE, CPU usage, throughput
- Does OVN support GRE natively? (ovn-controller tunnel types)

**Method:**
- Review Mellanox/Intel documentation
- Test GRE offload in lab if hardware available
- Benchmark EVPN vs GRE performance
- Document in `docs/GRE_OFFLOAD_ANALYSIS.md`

### Task 3: KubeVirt Migration API Research
**Goal:** Understand how to integrate with KubeVirt VM migration
**Questions:**
- How does KubeVirt trigger migration? (VirtualMachineInstanceMigration CRD?)
- What events can MCN agent watch? (VMI labels, status changes?)
- Does KubeVirt need changes to support MCN? Or can we integrate read-only?
- What's the migration timeline? (when to update network config?)

**Method:**
- Read KubeVirt migration documentation
- Test VM migration locally (Minikube + KubeVirt)
- Prototype MCN agent watching VMI CRs
- Document in `docs/KUBEVIRT_INTEGRATION.md`

### Task 4: OVN-K Managed VTEP Status
**Goal:** Understand upstream OVN-K managed VTEP roadmap
**Questions:**
- Is managed VTEP on OVN-K roadmap? Timeline?
- What's missing? (loopback IP assignment, EVPN controller integration?)
- Can we contribute the missing pieces?
- Should we wait for upstream or keep using unmanaged mode?

**Method:**
- Check OVN-K GitHub issues/PRs for managed VTEP
- Ask on OVN-K Slack/mailing list
- Review commit 357e39d5 and related PRs
- Document in `docs/MANAGED_VTEP_STATUS.md`

### Task 5: Route Reflector Topology Design
**Goal:** Design route reflector topology for large-scale deployments
**Questions:**
- When does full mesh not scale? (how many clusters?)
- How to select route reflectors? (dedicated nodes, control plane nodes?)
- How to configure FRR as route reflector vs client?
- Failover strategy if RR goes down?

**Method:**
- Review BGP route reflector best practices (RFC 4456)
- Test route reflector in 5-cluster setup
- Document in `docs/ROUTE_REFLECTOR_DESIGN.md`

---

## Open Questions for Community

1. **Naming:** What should the new repo be called?
   - `ovn-kubernetes/ovn-mcn` (OVN Multi-Cluster Networking)
   - `ovn-kubernetes/multi-cluster-networking`
   - `ovn-kubernetes/cluster-federation`
   - Other suggestions?

2. **Scope:** Should we also support default network stretching in future?
   - Security implications (cross-cluster pod communication on default network)
   - Use case: multi-cluster service mesh on default network
   - Or keep focused on UDNs only?

3. **Integration:** Should we integrate with Submariner for service discovery?
   - MCN handles network stretching (datapath)
   - Submariner handles service/DNS (control plane)
   - Complementary or overlap?

4. **IPAM:** Should IPAM be pluggable?
   - Allow external IPAM systems (e.g., Infoblox, NetBox)
   - Or keep broker-managed IPAM only?

5. **Transport:** What other transports are important?
   - IPsec overlay for security?
   - GENEVE (OVN native)?
   - WireGuard?
   - Or focus on EVPN + GRE for now?

6. **VM Migration:** Is live migration in scope, or just cold migration?
   - Live migration: Network continuity during migration (complex)
   - Cold migration: Network setup after migration (simpler)
   - What's the primary use case?

---

## Next Steps

### Immediate (This Week)
1. ✅ Create this planning document
2. 🔄 Review with team/stakeholders
3. 🔄 Start OKEP draft using this outline
4. 🔄 Begin research tasks (CUDN topology analysis)

### Short-term (Next 2 Weeks)
1. Complete OKEP draft (sections 1-6)
2. Complete research tasks 1-3
3. Share OKEP draft with OVN-K community for early feedback
4. Propose repo name and structure

### Medium-term (Next Month)
1. OKEP review and approval
2. New repo creation under ovn-kubernetes org
3. Start Phase 1 implementation (IPAM)
4. Set up CI/CD and testing infrastructure

---

## Resources

### Documentation
- [OVN-K CUDN Documentation](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/user-defined-networks.md)
- [Submariner Architecture](https://submariner.io/getting-started/architecture/)
- [Kubernetes Enhancement Proposal Template](https://github.com/kubernetes/enhancements/blob/master/keps/NNNN-kep-template/README.md)
- [KubeVirt Migration](https://kubevirt.io/user-guide/operations/live_migration/)

### Code References
- [Current PoC: yboaron/mcn (vtep_unmanaged branch)](https://github.com/yboaron/mcn/tree/vtep_unmanaged)
- [OVN-K VTEP Controller](https://github.com/ovn-org/ovn-kubernetes/tree/master/go-controller/pkg/crd/vtep)
- [OVN-K EVPN Controller](https://github.com/ovn-org/ovn-kubernetes/tree/master/go-controller/pkg/crd/evpn)

### Community
- OVN-K Community Meetings: Bi-weekly Thursdays
- OVN-K Slack: #ovn-kubernetes
- Meeting Notes: [Link to meeting doc]

---

**Document Owner:** Yossi Boaron  
**Last Updated:** 2026-05-12  
**Status:** Draft - Planning Phase
