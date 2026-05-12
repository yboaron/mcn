# MCN OKEP Research Tasks

**Purpose:** Track research needed to complete OKEP draft  
**Owner:** Yossi Boaron  
**Target:** Complete before OKEP review (2 weeks)

---

## Task Overview

| # | Task | Priority | Status | Owner | Due Date |
|---|------|----------|--------|-------|----------|
| 1 | CUDN Types & Topology Analysis | 🔴 High | 🔄 Not Started | - | 2026-05-16 |
| 2 | GRE Hardware Offload Research | 🟡 Medium | 🔄 Not Started | - | 2026-05-22 |
| 3 | KubeVirt Migration API Research | 🔴 High | 🔄 Not Started | - | 2026-05-23 |
| 4 | OVN-K Managed VTEP Status | 🟢 Low | 🔄 Not Started | - | 2026-05-26 |
| 5 | Route Reflector Topology Design | 🟡 Medium | 🔄 Not Started | - | 2026-05-24 |
| 6 | EVPN Type-2 Routes (L2) Validation | 🟡 Medium | 🔄 Not Started | - | 2026-05-24 |
| 7 | IPAM Controller Design | 🔴 High | 🔄 Not Started | - | 2026-05-19 |
| 8 | Cloud Platform Support | 🔴 High | 🔄 Not Started | - | 2026-05-25 |

---

## Task 1: CUDN Types & Topology Analysis 🔴 **HIGH PRIORITY**

### Goal
Understand all CUDN types (L2/L3, Primary/Secondary) and topology types (layer2/layer3/localnet), and how MCN can stretch each across clusters.

### Questions to Answer

**Topology Types:**
1. **What topology types does CUDN support?**
   - `localnet` - What is it? How does it work?
   - `layer2` - L2 switching, EVPN Type-2 routes
   - `layer3` - L3 routing, EVPN Type-5 routes (PoC already uses this)
   - Others? (Check OVN-K code)

2. **How does OVN-K implement each topology?**
   - OVN logical switch vs logical router
   - Localnet bridge connections
   - Port binding types
   - NAT, routing, switching rules

3. **Which topologies can be stretched? Which cannot?**
   - layer3: ✅ Working in PoC
   - layer2: ❓ Should work with EVPN Type-2, need to validate
   - localnet: ❓ External bridge dependency, how to stretch?

4. **What transport requirements does each have?**
   - layer3: EVPN Type-5 (IP prefix routes), GRE L3
   - layer2: EVPN Type-2 (MAC/IP routes), GRE L2, needs broadcast/multicast handling
   - localnet: ❓ Research needed

**CUDN Types (L2 vs L3):**
5. **What's the difference between L2 and L3 CUDNs?**
   - L2: Same broadcast domain, ARP works across clusters, MAC learning
   - L3: Routed connectivity, different broadcast domains per cluster

6. **When should we use L2 vs L3?**
   - L2 use cases: VM migration (L2 adjacency needed), legacy apps expecting broadcast
   - L3 use cases: Most modern microservices, better scalability

7. **Can we stretch both L2 and L3 CUDNs?**
   - L3: ✅ Working in PoC (layer3 topology)
   - L2: ❓ Need to validate EVPN Type-2 support

**CUDN Types (Primary vs Secondary):**
8. **What's `spec.network.role`?**
   - Primary: Can it be the default pod network? Security implications?
   - Secondary: Attached via Multus (standard for CUDNs)

9. **Can we stretch both Primary and Secondary networks?**
   - Secondary: ✅ Standard use case (Multus multi-network)
   - Primary: ❓ Security concerns (cross-cluster default network)

10. **Are there security implications for stretching Primary networks?**
    - Pods on default network across clusters can communicate
    - NetworkPolicy enforcement across clusters?
    - Recommendation: Support Secondary only initially?

### Methodology
1. **Read OVN-K documentation:**
   - `docs/user-defined-networks.md`
   - `docs/design/user-defined-networks.md`
   - CRD definitions in `go-controller/pkg/crd/userdefinednetwork/v1/types.go`

2. **Review OVN-K code:**
   - CUDN controller: `go-controller/pkg/controller/user-defined-network/`
   - Topology implementations
   - Logical switch/router creation logic

3. **Test each topology locally:**
   ```bash
   # Create layer3 CUDN (already working in PoC)
   kubectl apply -f examples/layer3-cudn.yaml
   
   # Create layer2 CUDN (validate L2 connectivity)
   kubectl apply -f examples/layer2-cudn.yaml
   
   # Create localnet CUDN (understand external bridge requirement)
   kubectl apply -f examples/localnet-cudn.yaml
   ```

4. **Document findings:**
   - Create `docs/CUDN_TOPOLOGY_ANALYSIS.md`
   - Include topology comparison table
   - Note stretching feasibility for each
   - Recommend MCN support matrix

### Deliverables
- [ ] `docs/CUDN_TOPOLOGY_ANALYSIS.md` document
  - Topology support matrix table (layer2/layer3/localnet)
  - OVN implementation details
  - Stretching feasibility
  - Transport requirements (EVPN Type-2/Type-5, GRE L2/L3)
  - Recommendations for Phase 1/2/3 support

- [ ] `docs/CUDN_TYPES_ANALYSIS.md` document
  - L2 vs L3 comparison and use cases
  - Primary vs Secondary network analysis
  - Security implications
  - Support matrix for Phase 1/2/3

- [ ] Test YAML examples for each topology and type

### Status
🔄 **Not Started**

---

## Task 2: GRE Hardware Offload Research 🟡 **MEDIUM PRIORITY**

### Goal
Understand GRE hardware offload capabilities and how to implement GRE transport in MCN.

### Questions to Answer
1. **Which NICs support GRE offload?**
   - Mellanox BlueField-2/3
   - Intel E810
   - Broadcom NICs
   - Others?

2. **How to enable GRE offload?**
   - Kernel configuration (CONFIG_GRE_OFFLOAD)
   - ethtool commands (`ethtool -k eth0 | grep gre`)
   - NIC firmware requirements
   - OVS offload configuration

3. **Performance comparison: EVPN vs GRE**
   - CPU usage: VXLAN encap/decap vs GRE
   - Throughput: Software vs hardware offload
   - Latency impact
   - Memory bandwidth

4. **OVN GRE support:**
   - Does ovn-controller support GRE tunnels?
   - How to configure: `ovn-nbctl set-option tunnel_encaps=gre`?
   - GRE tunnel creation (ovs-vsctl add-port)
   - GRE key management (tunnel IDs)

5. **MCN GRE transport design:**
   - How to discover remote endpoints? (From Cluster CR)
   - How to configure GRE tunnels? (OVN vs direct OVS)
   - How to advertise routes? (Static routes vs BGP-free)
   - Offload enablement in MCN agent

### Methodology
1. **Review hardware documentation:**
   - Mellanox OFED documentation on GRE offload
   - Intel E810 datasheet
   - Check NIC capabilities via ethtool

2. **Test GRE offload (if hardware available):**
   ```bash
   # Check GRE offload support
   ethtool -k eth0 | grep gre
   
   # Enable GRE offload
   ethtool -K eth0 tx-gre-segmentation on
   
   # Create GRE tunnel
   ip tunnel add gre1 mode gre remote 172.18.0.3 local 172.18.0.2
   ip link set gre1 up
   
   # Benchmark
   iperf3 -s  # Server
   iperf3 -c <remote-ip>  # Client
   ```

3. **Research OVN GRE support:**
   - Check OVN documentation: `man ovn-northd`, `man ovn-controller`
   - Look for GRE examples in OVN code
   - Test GRE configuration in local OVN setup

4. **Benchmark EVPN vs GRE:**
   - Use current PoC with EVPN (baseline)
   - Implement simple GRE tunnel
   - Measure: throughput, CPU, latency
   - Document results

### Deliverables
- [ ] `docs/GRE_OFFLOAD_ANALYSIS.md` document
- [ ] NIC compatibility matrix (which NICs support GRE offload)
- [ ] Performance benchmarks (EVPN vs GRE)
- [ ] OVN GRE configuration guide
- [ ] GRE transport design proposal for MCN

### Status
🔄 **Not Started**

---

## Task 3: KubeVirt Migration API Research 🔴 **HIGH PRIORITY**

### Goal
Understand KubeVirt VM migration API and how MCN can integrate to support VM live migration.

### Questions to Answer
1. **How does KubeVirt trigger migration?**
   - `VirtualMachineInstanceMigration` CRD
   - API call to initiate migration
   - Source and target cluster specification

2. **What events can MCN agent watch?**
   - Watch VirtualMachineInstance (VMI) CRs?
   - Watch VMI labels or annotations?
   - Watch VirtualMachineInstanceMigration CRs?
   - Events or polling?

3. **Migration timeline - when to update network?**
   ```
   T0: Migration initiated
   T1: Target cluster starts preparing (VMI created on target)
   T2: Memory pre-copy begins
   T3: Final cutover (network needs to switch)
   T4: Source VM stopped, target VM running
   ```
   - When should MCN ensure network is ready on target?
   - When should MCN update routes (EVPN/GRE)?

4. **Does KubeVirt need changes to support MCN?**
   - Can MCN integrate read-only (watch CRs)?
   - Or does KubeVirt need to call MCN API?
   - Does multi-cluster migration work out-of-box?

5. **OVN-K logical port migration:**
   - How does VM IP stay the same across clusters?
   - OVN logical port created on target cluster before migration?
   - Who creates the port - KubeVirt or OVN-K?
   - Port migration vs new port creation?

### Methodology
1. **Read KubeVirt documentation:**
   - [Live Migration](https://kubevirt.io/user-guide/operations/live_migration/)
   - [Migration API](https://kubevirt.io/api-reference/master/definitions.html#_v1_virtualmachineinstancemigration)
   - Multi-cluster considerations

2. **Set up test environment:**
   ```bash
   # Install KubeVirt on existing Kind cluster
   kubectl apply -f https://github.com/kubevirt/kubevirt/releases/download/v1.0.0/kubevirt-operator.yaml
   kubectl apply -f https://github.com/kubevirt/kubevirt/releases/download/v1.0.0/kubevirt-cr.yaml
   
   # Create test VM
   kubectl apply -f examples/test-vm.yaml
   
   # Trigger migration
   virtctl migrate test-vm
   ```

3. **Watch migration events:**
   ```bash
   # Watch VMI status
   kubectl get vmi test-vm -w
   
   # Watch migration CR
   kubectl get virtualmachineinstancemigration -w
   
   # Check VMI events
   kubectl describe vmi test-vm
   ```

4. **Prototype MCN integration:**
   - Write simple controller to watch VMI CRs
   - Detect when VMI is being migrated
   - Log migration phases
   - Identify when to trigger network setup

5. **Engage with KubeVirt community:**
   - Post question on KubeVirt Slack/mailing list
   - Ask about multi-cluster migration support
   - Share MCN use case, get feedback

### Deliverables
- [ ] `docs/KUBEVIRT_INTEGRATION.md` document
- [ ] Migration timeline diagram (when MCN acts)
- [ ] Prototype controller (watch VMI, detect migration)
- [ ] Integration design proposal
- [ ] Open questions for KubeVirt SIG

### Status
🔄 **Not Started**

---

## Task 4: OVN-K Managed VTEP Status 🟢 **LOW PRIORITY**

### Goal
Understand upstream OVN-K managed VTEP roadmap and decide if MCN should wait or workaround.

### Questions to Answer
1. **Is managed VTEP on OVN-K roadmap?**
   - Check OVN-K GitHub issues/PRs
   - Ask on OVN-K Slack
   - Community meeting discussion

2. **What's missing in managed VTEP implementation?**
   - Loopback IP assignment (confirmed missing in vtep_managed test)
   - VTEP controller doesn't configure node loopback
   - EVPN controller doesn't integrate with managed IPs

3. **Timeline for completion?**
   - Is there active work on this?
   - Target OVN-K version?
   - Can we help/contribute?

4. **Should MCN wait or workaround?**
   - **Option 1:** Use unmanaged VTEP mode (manual IP assignment)
   - **Option 2:** Contribute managed VTEP implementation to OVN-K
   - **Option 3:** MCN agent assigns loopback IPs (workaround)

### Methodology
1. **Review OVN-K managed VTEP history:**
   ```bash
   # Check commit that disabled managed VTEP
   git log --grep="managed.*VTEP" --oneline
   
   # Look at commit 357e39d5
   git show 357e39d5
   ```

2. **Search GitHub issues/PRs:**
   - Search: "managed VTEP"
   - Check PR #6353 (original implementation)
   - Look for follow-up PRs

3. **Ask OVN-K community:**
   - Post on Slack: "What's the status of managed VTEP mode?"
   - Bring up in community meeting
   - Email ovn-k mailing list

4. **Evaluate contribution feasibility:**
   - How complex is loopback IP assignment?
   - Can we implement it in MCN agent? (workaround)
   - Or contribute to OVN-K upstream? (proper fix)

### Deliverables
- [ ] `docs/MANAGED_VTEP_STATUS.md` document
- [ ] Summary of upstream status and timeline
- [ ] Recommendation: wait, workaround, or contribute
- [ ] Contribution plan (if we decide to implement)

### Status
🔄 **Not Started**

---

## Task 5: Route Reflector Topology Design 🟡 **MEDIUM PRIORITY**

### Goal
Design route reflector topology for large-scale MCN deployments (>10 clusters).

### Questions to Answer
1. **When does full mesh not scale?**
   - Full mesh BGP sessions = N*(N-1)/2
   - 10 clusters = 45 sessions (manageable)
   - 20 clusters = 190 sessions (getting high)
   - 50 clusters = 1225 sessions (not scalable)

2. **Route reflector topology options:**
   - **Option 1:** Dedicated RR cluster (broker cluster acts as RR)
   - **Option 2:** RR on control plane nodes
   - **Option 3:** Hierarchical RR (edge → regional RR → global RR)

3. **How to configure FRR as RR vs client?**
   ```
   # RR config
   router bgp 64512
     neighbor 172.18.0.3 route-reflector-client
   
   # Client config
   router bgp 64513
     neighbor 172.18.0.2 remote-as 64512
   ```

4. **Failover strategy:**
   - If RR goes down, what happens?
   - Multiple RRs for redundancy?
   - How to detect RR failure and reconfigure?

5. **MCN agent RR support:**
   - How does agent know to configure RR vs client?
   - Cluster CR annotation? (`mcn.ovn.org/route-reflector: "true"`)
   - FRRConfiguration template for RR vs client

### Methodology
1. **Review BGP RR best practices:**
   - RFC 4456 (BGP Route Reflection)
   - FRR documentation on route reflectors
   - Real-world examples (Calico, Cilium BGP)

2. **Test route reflector in lab:**
   ```bash
   # 3-cluster setup: cluster1 = RR, cluster2/3 = clients
   
   # Cluster1 (RR) FRRConfiguration
   router bgp 64512
     bgp router-id 172.18.0.2
     neighbor 172.18.0.4 remote-as 64513
     neighbor 172.18.0.4 route-reflector-client
     neighbor 172.18.0.6 remote-as 64514
     neighbor 172.18.0.6 route-reflector-client
   
   # Cluster2 (client)
   router bgp 64513
     bgp router-id 172.18.0.4
     neighbor 172.18.0.2 remote-as 64512
   ```

3. **Measure convergence time:**
   - Baseline: Full mesh (3 clusters)
   - RR topology: 5 clusters
   - Measure: BGP route propagation time
   - Compare: Full mesh vs RR

4. **Design MCN agent RR logic:**
   - Cluster CR has `routeReflector: true` annotation
   - Agent detects RR role, configures FRR accordingly
   - Clients point to RR instead of full mesh

### Deliverables
- [ ] `docs/ROUTE_REFLECTOR_DESIGN.md` document
- [ ] Topology diagrams (full mesh vs RR)
- [ ] RR vs client FRRConfiguration templates
- [ ] Convergence time benchmarks
- [ ] MCN agent RR implementation plan

### Status
🔄 **Not Started**

---

## Task 6: EVPN Type-2 Routes (L2) Validation 🟡 **MEDIUM PRIORITY**

### Goal
Validate that OVN-K EVPN controller properly advertises EVPN Type-2 routes for layer2 CUDNs.

### Questions to Answer
1. **Does OVN-K support EVPN Type-2?**
   - Check ovn-bgp-agent or OVN-K EVPN controller code
   - Type-2 routes advertise MAC/IP pairs
   - Required for L2 CUDN stretching

2. **How to configure layer2 CUDN for EVPN?**
   - Same as layer3 (VNI, RT in CUDN spec)?
   - Or different RouteAdvertisement config?

3. **Broadcast/multicast handling:**
   - How does EVPN handle ARP broadcast?
   - BUM (Broadcast, Unknown unicast, Multicast) traffic
   - EVPN Type-3 routes (Inclusive Multicast)?

4. **Testing L2 connectivity:**
   - Can pods in different clusters ARP for each other?
   - Ping test using ICMP (layer3) works, but what about layer2?
   - Test with tools that require L2 adjacency

### Methodology
1. **Review OVN-K EVPN controller code:**
   - `go-controller/pkg/crd/evpn/`
   - Check if Type-2 routes are supported
   - Look for MAC advertisement logic

2. **Create layer2 CUDN:**
   ```yaml
   apiVersion: k8s.ovn.org/v1
   kind: ClusterUserDefinedNetwork
   metadata:
     name: test-layer2
   spec:
     network:
       subnets: ["192.168.100.0/24"]
     topology: layer2  # ← L2 topology
   ```

3. **Stretch layer2 CUDN across clusters:**
   - Create MCNC with layer2 CUDN
   - Check FRR routes: `vtysh -c 'show bgp l2vpn evpn'`
   - Look for Type-2 routes (MAC/IP advertisements)

4. **Test L2 connectivity:**
   ```bash
   # Deploy pods on layer2 network
   kubectl apply -f test-pods-layer2.yaml
   
   # Check ARP table on pod1
   kubectl exec pod1 -- ip neigh
   
   # Ping pod2 on different cluster
   kubectl exec pod1 -- ping <pod2-ip>
   
   # Test L2-specific protocol (e.g., DHCP, ARP scan)
   kubectl exec pod1 -- arping <pod2-ip>
   ```

5. **Document findings:**
   - Does EVPN Type-2 work out-of-box?
   - Any OVN-K gaps for L2 stretching?
   - Recommendations for MCN layer2 support

### Deliverables
- [ ] `docs/EVPN_TYPE2_VALIDATION.md` document
- [ ] layer2 CUDN test results
- [ ] FRR route dumps showing Type-2 routes
- [ ] L2 connectivity test report
- [ ] Gaps or issues found

### Status
🔄 **Not Started**

---

## Task 7: IPAM Controller Design 🔴 **HIGH PRIORITY**

### Goal
Design broker-side IPAM controller for allocating non-overlapping CIDRs across clusters.

### Questions to Answer
1. **IPAM allocation algorithm:**
   - User provides desired CIDR (e.g., 10.100.0.0/16)
   - How to split into non-overlapping subnets?
   - Example: /16 pool → /17 per cluster (2 clusters) or /18 per cluster (4 clusters)
   - Dynamic allocation vs pre-configured blocks?

2. **Conflict detection:**
   - What if cluster1 wants 10.100.0.0/16 but cluster2 wants 192.168.0.0/16?
   - Different pools → allocate independently
   - Same pool → allocate from same range, non-overlapping

3. **Optimistic locking:**
   - How to prevent race conditions? (Two agents allocating same CIDR)
   - Use MCN CR resourceVersion?
   - Retry logic on conflict?

4. **CIDR re-allocation:**
   - What if cluster leaves and rejoins?
   - Reuse same CIDR or allocate new?
   - How to handle CIDR pool exhaustion?

5. **Storage:**
   - Where to store allocations? (MultiClusterNetwork status?)
   - Example:
     ```yaml
     status:
       allocations:
         cluster1:
           cidr: 10.100.0.0/17
           allocatedAt: "2026-05-12T10:35:00Z"
         cluster2:
           cidr: 10.100.128.0/17
           allocatedAt: "2026-05-12T10:36:00Z"
     ```

### Methodology
1. **Review existing allocators in PoC:**
   - `pkg/agent/allocator/asn_allocator.go` (ASN allocation)
   - `pkg/agent/allocator/vni_allocator.go` (VNI allocation)
   - `pkg/agent/allocator/vtep_allocator.go` (VTEP CIDR allocation)
   - Use similar pattern for CIDR allocation

2. **Design IPAM controller:**
   ```go
   // pkg/broker/ipam/ipam_controller.go
   type IPAMController struct {
       client dynamic.Interface
   }
   
   func (c *IPAMController) AllocateCIDR(ctx context.Context, mcn *MultiClusterNetwork, clusterID string, desiredCIDR string) (string, error) {
       // 1. Parse desired CIDR
       // 2. Check if MCN has CIDR pool allocated
       // 3. Allocate non-overlapping subnet from pool
       // 4. Update MCN status with allocation
       // 5. Return allocated CIDR
   }
   ```

3. **CIDR splitting algorithm:**
   ```go
   // Split 10.100.0.0/16 into N equal subnets
   func splitCIDR(pool string, numClusters int) ([]string, error) {
       // Example: /16 → /17 for 2 clusters
       //          /16 → /18 for 4 clusters
       //          /16 → /19 for 8 clusters
   }
   ```

4. **Test scenarios:**
   - Single cluster: Allocate full CIDR
   - Two clusters: Allocate /17 each
   - Four clusters: Allocate /18 each
   - Conflict: Two agents request same CIDR simultaneously
   - Exhaustion: More clusters than subnets available

### Deliverables
- [ ] `docs/IPAM_CONTROLLER_DESIGN.md` document
- [ ] IPAM controller prototype code
- [ ] CIDR allocation algorithm
- [ ] Unit tests for IPAM logic
- [ ] Integration test plan

### Status
🔄 **Not Started**

---

## Task 8: Cloud Platform Support 🔴 **HIGH PRIORITY**

### Goal
Investigate stretching CUDNs across different cloud platforms (AWS, GCP, Azure, on-premises).

### Questions to Answer
1. **Can MCN stretch CUDNs when clusters run on different cloud providers?**
   - Example: Cluster1 on AWS EKS, Cluster2 on GCP GKE
   - Example: Cluster1 on Azure AKS, Cluster2 on on-premises datacenter
   - What networking challenges exist?

2. **What connectivity options are available?**
   - **VPN Tunnels:** AWS VPN ↔ GCP VPN, Azure VPN Gateway
   - **Cloud Interconnects:** AWS Direct Connect ↔ GCP Cloud Interconnect, Azure ExpressRoute
   - **Public Internet:** Nodes with public IPs (security concerns)

3. **Cloud-specific networking requirements:**
   - **AWS:** Security groups for BGP (TCP 179), VXLAN (UDP 4789), GRE (Protocol 47)
   - **GCP:** Firewall rules for BGP, VXLAN, GRE
   - **Azure:** Network Security Groups (NSGs) for MCN traffic

4. **IP addressing challenges:**
   - Can clouds allow custom VTEP IPs (100.0.0.0/8)?
   - Overlapping VPC CIDRs between clouds?
   - Public vs private IPs for BGP peering endpoints?

5. **Performance and cost:**
   - Expected latency (AWS us-east ↔ GCP us-central: ~20-30ms)
   - MTU considerations (VPN overhead + VXLAN/GRE)
   - Inter-cloud data transfer costs ($0.01-0.09/GB)

### Method
1. **Research cloud networking:**
   - AWS: VPN Gateway, Transit Gateway, Direct Connect, Security Groups
   - GCP: Cloud VPN, Cloud Interconnect, Cloud Router, Firewall Rules
   - Azure: VPN Gateway, ExpressRoute, Virtual WAN, NSGs
   - Review multi-cloud networking patterns (Aviatrix, service mesh)

2. **Document connectivity options:**
   - VPN tunnel setup between AWS and GCP
   - Security group / firewall rule requirements
   - MTU calculations (VPN overhead + encapsulation)
   - Latency expectations for common cloud pairs

3. **Create support matrix:**
   - Which cloud combinations are feasible?
   - Recommended connectivity method for each
   - Known limitations and workarounds
   - Cost implications

4. **Design test plan:**
   - Test scenarios for AWS + GCP
   - Test scenarios for On-prem + Cloud
   - Security considerations (IPsec overlay?)

### Deliverables
- [ ] `docs/CLOUD_PLATFORM_NETWORKING.md` document
  - Multi-cloud connectivity options
  - Security group / firewall requirements per cloud
  - VPN tunnel setup guide
  - MTU and latency considerations
  - Cost analysis

- [ ] `docs/CLOUD_PLATFORM_SUPPORT_MATRIX.md` document
  - Cloud combination support matrix
  - Recommended connectivity per combination
  - Known limitations and challenges
  - Phase 1/2/3 support roadmap

- [ ] Test plan for multi-cloud validation (Phase 2)

### Status
🔄 **Not Started**

---

## Summary Table

| Task | Priority | Complexity | Estimated Time | Dependencies |
|------|----------|------------|----------------|--------------|
| 1. CUDN Types & Topology | 🔴 High | Medium | 4 days | None |
| 2. GRE Offload Research | 🟡 Medium | Medium | 4 days | None |
| 3. KubeVirt Integration | 🔴 High | High | 5 days | None |
| 4. Managed VTEP Status | 🟢 Low | Low | 1 day | None |
| 5. Route Reflector Design | 🟡 Medium | Medium | 3 days | Task 1 |
| 6. EVPN Type-2 Validation | 🟡 Medium | Medium | 2 days | Task 1 |
| 7. IPAM Controller Design | 🔴 High | High | 3 days | None |
| 8. Cloud Platform Support | 🔴 High | Medium | 2 days | None |

**Total Estimated Time:** ~24 days (3-4 weeks with parallel work)

---

## Recommended Timeline

### Week 1 (May 12-16)
- **Start:** Task 1 (CUDN Topology), Task 7 (IPAM Design)
- **Goal:** Understand CUDN types, draft IPAM design

### Week 2 (May 19-23)
- **Start:** Task 3 (KubeVirt Integration), Task 2 (GRE Research)
- **Goal:** KubeVirt prototype, GRE feasibility

### Week 3 (May 26-30)
- **Start:** Task 5 (Route Reflector), Task 6 (EVPN Type-2)
- **Goal:** Scale design, L2 validation

### Week 4 (Jun 2-6)
- **Finish:** All tasks
- **Goal:** Complete all research, finalize OKEP draft

---

## Notes

- **Parallel work:** Tasks 1, 2, 3, 7 can be done in parallel (no dependencies)
- **Blocking tasks:** Task 1 should complete before Task 5 and 6 (need to understand topologies first)
- **High priority:** Focus on Tasks 1, 3, 7 first (critical for OKEP)
- **Medium priority:** Tasks 2, 5, 6 can wait until Week 2-3
- **Low priority:** Task 4 is nice-to-have, not blocking OKEP

---

**Last Updated:** 2026-05-12  
**Next Review:** 2026-05-19 (check progress on Week 1 tasks)
