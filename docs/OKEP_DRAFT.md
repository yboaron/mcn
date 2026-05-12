# OKEP: Multi-Cluster Networking for OVN-Kubernetes

**OKEP Number:** TBD  
**Title:** Multi-Cluster Networking for OVN-Kubernetes  
**Authors:** Yossi Boaron (@yboaron)  
**Status:** Provisional  
**Created:** 2026-05-12  
**Target OVN-K Version:** v24.09+

---

## Table of Contents

- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
  - [Use Cases](#use-cases)
- [Proposal](#proposal)
  - [Architecture Overview](#architecture-overview)
  - [User Stories](#user-stories)
  - [API Design](#api-design)
- [Design Details](#design-details)
  - [IPAM Coordination](#ipam-coordination)
  - [Transport Abstraction](#transport-abstraction)
  - [CUDN Type Support](#cudn-type-support)
  - [VM Live Migration](#vm-live-migration)
- [Risks and Mitigations](#risks-and-mitigations)
- [Test Plan](#test-plan)
- [Graduation Criteria](#graduation-criteria)
- [Implementation History](#implementation-history)
- [Alternatives](#alternatives)

---

## Summary

This proposal introduces a **multi-cluster networking layer** for OVN-Kubernetes that enables User Defined Networks (UDNs) to be stretched across multiple Kubernetes clusters. The solution uses an external **broker-agent architecture** (similar to Submariner) to orchestrate network configuration, BGP peering, IPAM coordination, and transport setup across clusters.

**Key capabilities:**
- Stretch ClusterUserDefinedNetworks (CUDNs) across multiple clusters
- Support multiple transport mechanisms (EVPN, GRE, others)
- Coordinate IPAM to prevent CIDR conflicts across clusters
- Enable VM live migration across clusters (KubeVirt integration)
- Support all CUDN types: Primary/Secondary, L2/L3, all topology modes
- Work with upstream OVN-K without forking (use existing CRDs)

---

## Motivation

### Goals

1. **Enable CUDN stretching** across multiple Kubernetes clusters with minimal user configuration
2. **Support multiple transports:**
   - EVPN (BGP-based) for standard deployments
   - GRE for hardware offload optimization
   - Extensible architecture for future transports (GENEVE, IPsec, WireGuard)
3. **Provide cross-cluster IPAM** to automatically allocate non-overlapping CIDRs for same CUDN across clusters
4. **Enable VM Live Migration** across clusters with network continuity (KubeVirt integration)
5. **Support all CUDN types:**
   - Primary and Secondary networks
   - L2 (layer2) and L3 (layer3) topologies
   - All topology modes (localnet, layer2, layer3)
6. **Maintain OVN-K compatibility:**
   - Work with upstream OVN-K (no forking)
   - Use existing CRDs where possible (VTEP, CUDN, RouteAdvertisement, FRRConfiguration)
   - External orchestration layer (non-invasive)
7. **Production-ready:**
   - Scale to 10+ clusters
   - Route reflector topology for large deployments
   - Comprehensive testing and observability

### Non-Goals

The following are **explicitly out of scope** for this proposal:

1. **Default network stretching** - Security and isolation concerns make stretching the default pod network across clusters risky. Focus is on User Defined Networks only.
2. **Cross-cluster Service discovery** - Handled by other projects (Submariner, CoreDNS multi-cluster plugins). MCN focuses on network datapath, not service APIs.
3. **Multi-cluster policy federation** - NetworkPolicy enforcement across clusters is future work, not part of initial OKEP.
4. **Cross-cluster DNS** - Delegate to existing solutions (Submariner Lighthouse, external DNS).
5. **Gateway-based tunneling** - MCN uses full-mesh connectivity (all nodes participate), not gateway nodes like Submariner.

### Use Cases

#### UC1: Disaster Recovery / Geographic Redundancy

**Actor:** Platform operator managing multi-region Kubernetes deployment  
**Goal:** Deploy applications across geographically distributed clusters for fault tolerance

**Scenario:**
- Three Kubernetes clusters: US-East, US-West, EU-Central
- Application "payment-service" uses CUDN "payment-network" (10.200.0.0/16)
- Operator stretches "payment-network" across all three clusters
- Payment pods can run in any cluster, communicate via stretched network
- If US-East fails, workload fails over to US-West with same network identity

**Requirements:**
- CUDN stretching with IPAM coordination (each cluster gets non-overlapping CIDR)
- BGP/EVPN peering between regions
- Automatic failover without changing pod network config

**Success Criteria:**
- Pod in US-East (10.200.0.5) can reach pod in EU-Central (10.200.128.10)
- Failover takes <60 seconds (time for OVN-K to schedule pods on new cluster)

---

#### UC2: Workload Migration / Bursting

**Actor:** Application developer running batch processing workloads  
**Goal:** Migrate workloads between clusters based on resource availability and cost

**Scenario:**
- Two clusters: On-prem (expensive, limited capacity) and Cloud (cheaper, elastic)
- Batch job uses CUDN "batch-network" for inter-process communication
- During peak load, migrate some batch pods from on-prem to cloud
- Pods maintain network connectivity during migration
- After peak, migrate pods back to on-prem to reduce cloud costs

**Requirements:**
- CUDN stretched across on-prem and cloud clusters
- Pod-to-pod connectivity regardless of pod location
- No application changes needed (same network identity)

**Success Criteria:**
- Migrate 100 pods from on-prem to cloud without network disruption
- Network latency on-prem to cloud <50ms (depends on WAN)
- Application continues processing without errors

---

#### UC3: VM Live Migration Across Clusters ⭐ **Community Priority**

**Actor:** Virtualization platform operator using KubeVirt  
**Goal:** Live migrate VirtualMachines between clusters while maintaining network connectivity

**Scenario:**
- Two clusters: Cluster-A (older hardware) and Cluster-B (newer GPUs)
- VMs use CUDN "vm-network" (L2 connectivity, 192.168.100.0/24)
- VM "gpu-workload" on Cluster-A needs GPU upgrade
- Live migrate VM to Cluster-B without downtime
- VM keeps same IP (192.168.100.50) during and after migration
- L2 adjacency maintained (ARP, broadcast) if needed

**Requirements:**
- L2 CUDN stretching (layer2 topology) for VM network continuity
- EVPN Type-2 routes for MAC/IP advertisement
- Integration with KubeVirt migration API (detect migration events)
- Sub-second network disruption during cutover

**Success Criteria:**
- VM migrates from Cluster-A to Cluster-B in <10 seconds
- Network disruption <1 second (time for EVPN route convergence)
- VM application (e.g., database) maintains connections without errors
- VM IP and MAC remain unchanged

**Open Questions:**
- Does MCN agent need to actively participate in migration, or just ensure network is ready?
- How to coordinate with KubeVirt migration controller? (Watch VirtualMachineInstanceMigration CRD?)
- Does OVN-K need changes to support VM migration? (Logical port migration between clusters?)

---

#### UC4: Multi-Cluster Service Mesh

**Actor:** Service mesh operator running Istio  
**Goal:** Extend service mesh sidecar network across multiple clusters

**Scenario:**
- Three clusters in different regions
- Istio service mesh uses CUDN "istio-sidecar-network" for control plane
- Stretch sidecar network across clusters for unified mesh
- Pods in any cluster can communicate via mesh with mTLS
- Centralized Istio control plane in one cluster

**Requirements:**
- L3 CUDN stretching (layer3 topology)
- Support for secondary networks (Istio sidecar is secondary interface)
- Encrypted transport (EVPN over IPsec or separate IPsec transport)

**Success Criteria:**
- Istio sidecar pods in all clusters join single mesh
- Cross-cluster mTLS connections work
- Istio metrics and tracing span all clusters

---

#### UC5: Edge / Hub-and-Spoke Deployments

**Actor:** Edge computing platform operator  
**Goal:** Connect edge clusters to central hub via stretched networks

**Scenario:**
- 1 hub cluster (central datacenter) + 10 edge clusters (stores, factories, etc.)
- Each edge cluster has local workloads
- Hub cluster runs aggregation, monitoring, control services
- CUDN "edge-aggregation" connects all edge clusters to hub
- Edge devices send data to hub over stretched network

**Requirements:**
- Hub-and-spoke topology (not full mesh between edge clusters)
- Route reflector on hub to avoid full BGP mesh
- Support for WAN links (higher latency, lower bandwidth)

**Success Criteria:**
- 10 edge clusters + 1 hub, all connected via MCN
- BGP convergence <30 seconds when edge cluster joins
- Data flows from edge to hub without errors

---

#### UC6: Hardware Offload Optimization ⭐ **Community Priority**

**Actor:** Cloud provider with SmartNIC hardware  
**Goal:** Use GRE transport with hardware offload instead of EVPN for higher throughput

**Scenario:**
- Two clusters with Mellanox BlueField-2 SmartNICs
- Workload requires high-throughput network (AI training, storage replication)
- EVPN (VXLAN) over BGP uses CPU for encap/decap
- GRE tunnels can be offloaded to NIC hardware → 10x throughput improvement
- Stretch CUDN using GRE transport instead of EVPN

**Requirements:**
- Transport abstraction (select GRE instead of EVPN)
- GRE tunnel configuration via OVN or direct (ovs-vsctl)
- Hardware offload enablement (ethtool, NIC firmware)
- Performance validation (benchmark EVPN vs GRE)

**Success Criteria:**
- CUDN stretched using GRE transport
- Hardware offload enabled and verified (ethtool -k shows "tx-gre-segmentation: on")
- Throughput 10Gbps+ with <10% CPU usage (vs 5Gbps with 50% CPU for EVPN)

**Open Questions:**
- Does OVN support GRE tunnels natively? (ovn-controller tunnel types)
- How to configure GRE in OVN? (ovn-nbctl set-option tunnel_encaps=gre)
- Which NICs support GRE offload? (Mellanox BlueField, Intel E810, others?)

---

#### UC7: Non-Overlapping CIDR Coordination ⭐ **Community Priority**

**Actor:** Multi-cluster platform operator  
**Goal:** Create same CUDN in multiple clusters with automatically coordinated, non-overlapping CIDRs

**Scenario:**
- Operator creates CUDN "blue-network" in Cluster-1 (desires 10.100.0.0/16)
- Operator creates CUDN "blue-network" in Cluster-2 (also desires 10.100.0.0/16)
- Without coordination: Both clusters allocate 10.100.0.0/16 → CIDR conflict when stretching!
- With MCN IPAM: Broker allocates non-overlapping subnets automatically:
  - Cluster-1: 10.100.0.0/17 (10.100.0.0 - 10.100.127.255)
  - Cluster-2: 10.100.128.0/17 (10.100.128.0 - 10.100.255.255)
- Same CUDN name, non-overlapping CIDRs, seamless stretching

**Requirements:**
- Broker-managed IPAM service
- CIDR allocation from user-specified pool (e.g., 10.100.0.0/16)
- Conflict detection and resolution
- Support for CIDR re-allocation if cluster leaves/joins

**Success Criteria:**
- User creates CUDN with desired CIDR (e.g., 10.100.0.0/16)
- MCN IPAM allocates non-overlapping subnet automatically
- User doesn't need to manually coordinate CIDRs across clusters
- Clear error message if desired CIDR conflicts with existing allocation

**Implementation Note:**
- Similar to ASN/VNI allocators in current PoC
- Use optimistic locking for conflict-free allocation
- Store allocation in MultiClusterNetwork status: `status.allocations[clusterID].cidr`

---

## Proposal

### Architecture Overview

MCN uses a **broker-agent pattern** inspired by Submariner, where the broker is pure storage and agents do all orchestration:

```
                   ┌─────────────────────────────────────┐
                   │         Broker Cluster              │
                   │  (Just Kubernetes API + CRDs)       │
                   │                                     │
                   │  ┌───────────────────────────┐     │
                   │  │  MultiClusterNetwork CRs  │     │
                   │  │  - VNI: 5001              │     │
                   │  │  - CIDR Pool: 10.100.0.0/16│    │
                   │  │  - Participants: [c1, c2] │     │
                   │  └───────────────────────────┘     │
                   │  ┌───────────────────────────┐     │
                   │  │  Cluster CRs              │     │
                   │  │  - cluster1: ASN 64512    │     │
                   │  │  - cluster2: ASN 64513    │     │
                   │  └───────────────────────────┘     │
                   └──────────┬──────────────┬───────────┘
                              │              │
                    ┌─────────┴──────┐  ┌────┴──────────┐
                    │                │  │               │
         ┌──────────▼─────────┐  ┌──▼──▼────────────┐  │
         │   Cluster 1        │  │   Cluster 2      │  │
         │                    │  │                  │  │
         │  ┌──────────────┐  │  │  ┌────────────┐ │  │
         │  │  MCN Agent   │  │  │  │ MCN Agent  │ │  │
         │  │              │  │  │  │            │ │  │
         │  │ • Syncs CRs  │  │  │  │ • Syncs CRs│ │  │
         │  │ • Allocates  │  │  │  │ • Allocates│ │  │
         │  │ • Configures │  │  │  │ • Configs  │ │  │
         │  └──────┬───────┘  │  │  └─────┬──────┘ │  │
         │         │          │  │        │        │  │
         │  ┌──────▼───────┐  │  │  ┌─────▼──────┐ │  │
         │  │   OVN-K      │  │  │  │   OVN-K    │ │  │
         │  │ • VTEP       │  │  │  │ • VTEP     │ │  │
         │  │ • CUDN       │  │  │  │ • CUDN     │ │  │
         │  │ • RouteAdv   │  │  │  │ • RouteAdv │ │  │
         │  │ • FRRConfig  │  │  │  │ • FRRConfig│ │  │
         │  └──────────────┘  │  │  └────────────┘ │  │
         └────────────────────┘  └─────────────────┘  │
                                                       │
              BGP Peering (eBGP EVPN) ───────────────┘
```

**Components:**

1. **Broker Cluster:**
   - Standard Kubernetes cluster with MCN CRDs installed
   - Stores cluster registry (Cluster CRs) and network registry (MultiClusterNetwork CRs)
   - **No controllers** - just API storage (etcd)
   - Can be one of the managed clusters or dedicated cluster

2. **MCN Agent:**
   - Runs on each managed cluster (DaemonSet or Deployment)
   - Syncs cluster state to/from broker (Submariner Admiral)
   - Allocates resources (ASN, VNI, VTEP CIDR, CUDN CIDR via broker IPAM)
   - Configures OVN-K resources (VTEP, CUDN patches, RouteAdvertisement, FRRConfiguration)
   - Watches MultiClusterNetworkConnect CRs (user intent to stretch CUDN)

3. **CRDs:**

   **Broker CRDs (namespace-scoped):**
   - `Cluster`: Cluster registration with endpoints, ASN, VTEP CIDR
   - `MultiClusterNetwork`: Network registry with VNI, route target, CIDR pool, participants

   **Local CRDs (cluster-scoped):**
   - `MultiClusterNetworkConnect`: User intent to join local CUDN to multi-cluster network

4. **OVN-K Integration:**
   - Agent creates/updates OVN-K CRDs:
     - `VTEP`: Tunnel endpoints for remote clusters
     - `ClusterUserDefinedNetwork`: Patch with EVPN config (VNI, RT)
     - `RouteAdvertisement`: Advertise CUDN subnets via BGP
     - `FRRConfiguration`: BGP peering config (via FRR-K8s)

### User Stories

**Story 1: Stretch existing CUDN across two clusters**

```yaml
# Cluster-1: User creates CUDN (standard OVN-K)
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
spec:
  network:
    subnets: ["10.100.0.0/16"]  # Will be adjusted by MCN IPAM
  topology: layer3

# Cluster-1: User stretches CUDN (MCN-specific)
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  multiClusterNetwork: blue-network
  localNetwork: blue-network
  transport: evpn

# Cluster-2: User creates same CUDN and MCNC
# (Broker IPAM ensures non-overlapping CIDRs)
```

**Result:**
- Broker IPAM allocates:
  - Cluster-1: 10.100.0.0/17
  - Cluster-2: 10.100.128.0/17
- MCN agent patches CUDNs with allocated CIDRs
- BGP sessions established, EVPN routes exchanged
- Pods in blue-network on Cluster-1 can reach Cluster-2

---

**Story 2: Use GRE transport for hardware offload**

```yaml
# Same as Story 1, but with different transport
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  multiClusterNetwork: blue-network
  localNetwork: blue-network
  transport: gre  # ← Use GRE instead of EVPN
  transportConfig:
    offload: true  # Enable hardware offload if available
    ttl: 64
```

**Result:**
- MCN agent configures GRE tunnels instead of EVPN
- OVN tunnel type set to GRE
- Hardware offload enabled on NIC (if supported)
- Higher throughput, lower CPU usage vs EVPN

---

### API Design

#### Custom Resource Definitions

**1. Cluster (Broker CRD, Namespace-scoped)**

```yaml
apiVersion: multicluster.ovn.org/v1
kind: Cluster
metadata:
  name: cluster1
  namespace: mcn-broker
spec:
  clusterID: cluster1
  asn: 64512
  vtepCIDR: 100.0.0.0/16
  endpoints:
    - nodeID: cluster1-control-plane
      ip: 172.18.0.2
      vtepIP: 100.0.0.0
    - nodeID: cluster1-worker
      ip: 172.18.0.3
      vtepIP: 100.0.0.1
status:
  syncedAt: "2026-05-12T10:30:00Z"
  agentVersion: v0.1.0
```

**2. MultiClusterNetwork (Broker CRD, Namespace-scoped)**

```yaml
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetwork
metadata:
  name: blue-network
  namespace: mcn-broker
spec:
  vni: 5001
  routeTarget: "65000:5001"
  cidrPool: 10.100.0.0/16  # Pool to allocate from
  transport: evpn  # or gre, geneve, ipsec
status:
  participants:
    - clusterID: cluster1
      allocatedCIDR: 10.100.0.0/17
      joinedAt: "2026-05-12T10:35:00Z"
    - clusterID: cluster2
      allocatedCIDR: 10.100.128.0/17
      joinedAt: "2026-05-12T10:36:00Z"
```

**3. MultiClusterNetworkConnect (Local CRD, Cluster-scoped)**

```yaml
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  multiClusterNetwork: blue-network
  localNetwork: blue-network  # CUDN name
  transport: evpn
  transportConfig:
    # Transport-specific config (optional)
status:
  phase: Connected  # Pending, Connecting, Connected, Failed
  allocatedCIDR: 10.100.0.0/17
  allocatedVNI: 5001
  conditions:
    - type: IPAMAllocated
      status: "True"
      lastTransitionTime: "2026-05-12T10:35:05Z"
    - type: TransportConfigured
      status: "True"
      lastTransitionTime: "2026-05-12T10:35:10Z"
    - type: CUDNPatched
      status: "True"
      lastTransitionTime: "2026-05-12T10:35:15Z"
```

---

## Design Details

### IPAM Coordination

[See OKEP_PLANNING.md Section 5 for detailed design]

**Summary:**
- Broker-managed IPAM controller
- User specifies desired CIDR in CUDN spec
- Broker allocates non-overlapping subnet from pool
- Agent patches CUDN with allocated CIDR
- Optimistic locking prevents conflicts

### Transport Abstraction

[See OKEP_PLANNING.md Section 5 for transport plugin interface]

**Summary:**
- Pluggable transport architecture
- Initial support: EVPN (Phase 1), GRE (Phase 2)
- User selects transport in MCNC spec
- Agent loads appropriate transport plugin

### CUDN Type Support

[See OKEP_PLANNING.md Section 5 for topology support matrix]

**Summary:**
- Layer3 (L3 routing): Phase 1 ✅
- Layer2 (L2 switching): Phase 2 (validate EVPN Type-2)
- Localnet: Phase 3 (research needed)
- Primary and Secondary networks: Both supported

### VM Live Migration

[See OKEP_PLANNING.md Section 5 for migration flow]

**Summary:**
- Detect KubeVirt migration events
- Ensure CUDN stretched to destination cluster
- OVN-K handles logical port migration
- EVPN propagates route updates
- Sub-second network cutover

---

## Risks and Mitigations

[See OKEP_PLANNING.md Section 6 for full risk analysis]

**Key Risks:**
1. OVN-K managed VTEP incomplete → Use unmanaged mode
2. IPAM complexity → Comprehensive testing
3. Transport diversity → Phased approach (EVPN first, GRE second)
4. VM migration coordination → Early KubeVirt engagement
5. Scale limits → Route reflector topology

---

## Test Plan

[See OKEP_PLANNING.md Section 7 for full test plan]

**Test Levels:**
- Unit tests (IPAM, transport plugins)
- Integration tests (2-cluster stretching)
- E2E tests (multi-cluster, pod-to-pod)
- Scale tests (10 clusters, 50 CUDNs)

---

## Graduation Criteria

**Alpha:**
- OKEP approved
- Basic CUDN stretching (EVPN, layer3)
- Broker IPAM
- 2-cluster E2E tests

**Beta:**
- GRE transport
- Layer2 support
- VM migration
- 3+ cluster tests

**GA:**
- Production deployments
- Observability
- Security review
- Performance benchmarks

---

## Implementation History

- 2026-05-11: Presented PoC to OVN-K community, received approval for OKEP
- 2026-05-12: OKEP draft created
- TBD: OKEP review
- TBD: New repo creation
- TBD: Phase 1 implementation

---

## Alternatives

[See OKEP_PLANNING.md Section 10 for alternatives analysis]

**Alternatives Considered:**
1. In-tree OVN-K controllers (rejected - too invasive)
2. Use Submariner directly (rejected - different architecture)
3. No IPAM coordination (rejected - poor UX)
4. EVPN-only (hybrid - start with EVPN, add GRE later)

---

## Open Questions

1. Repository naming: `ovn-mcn`, `multi-cluster-networking`, or other?
2. Should we support default network stretching in future?
3. Integration with Submariner for service discovery?
4. Should IPAM be pluggable for external systems?
5. What other transports are priority? (IPsec, GENEVE, WireGuard)
6. Live migration vs cold migration - which is primary use case?

---

## References

- [PoC Repository](https://github.com/yboaron/mcn/tree/vtep_unmanaged)
- [PoC Presentation](https://github.com/yboaron/mcn/blob/vtep_unmanaged/docs/Stretching_CUDNs_across_clusters_PoC.pdf)
- [OVN-K CUDN Documentation](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/user-defined-networks.md)
- [Submariner Architecture](https://submariner.io/getting-started/architecture/)
- [KubeVirt Migration](https://kubevirt.io/user-guide/operations/live_migration/)

---

**Authors:** Yossi Boaron (@yboaron)  
**Reviewers:** TBD  
**Last Updated:** 2026-05-12  
**Status:** Draft
