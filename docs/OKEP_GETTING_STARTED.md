# Getting Started with OKEP Development

**Quick Start Guide for OVN-K Multi-Cluster Networking Enhancement Proposal**

---

## What We're Building

An **OVN-Kubernetes Enhancement Proposal (OKEP)** for multi-cluster networking that will:

1. ✅ Enable stretching CUDNs across multiple clusters
2. ✅ Support multiple transports (EVPN, GRE, others)
3. ✅ Provide cross-cluster IPAM (non-overlapping CIDRs)
4. ✅ Enable VM live migration across clusters
5. ✅ Create new repo under `ovn-kubernetes` organization

**Timeline:** Draft OKEP in 2 weeks → Community review → New repo → Phase 1 implementation

---

## What You Have Now

### ✅ Created Documents

1. **`OKEP_PLANNING.md`** - Comprehensive planning document
   - 10 sections covering all aspects
   - Use cases from community meeting
   - Architecture, API design, risks, test plan
   - Implementation phases (Phase 1-3)
   - Research tasks outlined

2. **`OKEP_DRAFT.md`** - Actual OKEP draft (Kubernetes KEP format)
   - Ready for community review
   - Sections: Summary, Motivation, Proposal, Design Details, etc.
   - Use cases written in story format
   - API examples
   - Graduation criteria (Alpha/Beta/GA)

3. **`RESEARCH_TASKS.md`** - Task tracker for research work
   - 7 research tasks with priorities
   - Estimated timeline (3-4 weeks)
   - Deliverables for each task
   - Status tracking

### ✅ What's Already Done

- ✅ Working PoC (vtep_unmanaged branch)
- ✅ Community presentation and approval
- ✅ Meeting feedback captured
- ✅ Use cases documented (7 use cases from community)
- ✅ Architecture designed (broker-agent pattern)
- ✅ OKEP structure ready

---

## Next Steps (Prioritized)

### 🔴 **Immediate (This Week - May 12-16)**

1. **Review planning documents:**
   - Read `OKEP_PLANNING.md` (comprehensive overview)
   - Read `OKEP_DRAFT.md` (actual OKEP)
   - Provide feedback, add missing points

2. **Start high-priority research:**
   - **Task 1:** CUDN Topology Analysis (understand layer2/layer3/localnet)
   - **Task 7:** IPAM Controller Design (critical for OKEP)
   
   **Why these first?**
   - CUDN topology knowledge needed for rest of OKEP
   - IPAM is a key differentiator (community raised CIDR conflicts)

3. **Refine use cases:**
   - Review UC3 (VM Live Migration) - community priority
   - Review UC6 (GRE Hardware Offload) - community priority  
   - Review UC7 (Non-Overlapping CIDR) - community priority
   - Add more details based on your domain knowledge

### 🟡 **Short-term (Week 2 - May 19-23)**

1. **Complete OKEP draft:**
   - Fill in missing sections based on research findings
   - Add diagrams (architecture, flows, topologies)
   - Write API reference section
   - Add code examples

2. **Continue research:**
   - **Task 3:** KubeVirt Migration API (VM migration use case)
   - **Task 2:** GRE Hardware Offload (alternative transport)

3. **Prepare for community review:**
   - Create presentation slides for OKEP (if needed)
   - Identify reviewers (OVN-K maintainers, community members)
   - Prepare FAQ for common questions

### 🟢 **Medium-term (Week 3-4 - May 26-Jun 6)**

1. **Submit OKEP for review:**
   - Post OKEP draft to OVN-K mailing list
   - Present in community meeting (if requested)
   - Incorporate feedback

2. **Finish remaining research:**
   - **Task 5:** Route Reflector Design (scale)
   - **Task 6:** EVPN Type-2 Validation (L2 support)
   - **Task 4:** Managed VTEP Status (optional)

3. **Propose new repo:**
   - Repo name: `ovn-kubernetes/ovn-mcn` (or community preference)
   - Initial structure, CI/CD, guidelines
   - Migration plan from yboaron/mcn PoC

---

## How to Use These Documents

### OKEP_PLANNING.md (Master Plan)

**Use for:**
- Understanding full scope of project
- Reference for design decisions
- Planning implementation phases
- Risk assessment

**Key sections:**
- Section 3 (Use Cases) - Share with stakeholders
- Section 5 (Design Details) - Reference during implementation
- Section 9 (Implementation Phases) - Project roadmap

### OKEP_DRAFT.md (Community-Facing)

**Use for:**
- Submitting to OVN-K community
- Review and feedback
- Official record of proposal

**Key sections:**
- Section 2 (Motivation) - Why we need this
- Section 3 (Proposal) - What we're building
- Section 4 (Design Details) - How it works

**Next steps for this doc:**
- Add diagrams (architecture, API flow)
- Fill in TODOs based on research findings
- Review for clarity and completeness

### RESEARCH_TASKS.md (Task Tracker)

**Use for:**
- Daily/weekly task planning
- Tracking research progress
- Ensuring nothing is missed

**How to update:**
- Mark tasks as "In Progress" when starting
- Update status column as you complete deliverables
- Add notes or blockers

---

## Quick Reference: Community Feedback

### What Community Wants

1. ✅ **Multi-cluster networking layer** on top of OVN-K (not in-tree)
2. ✅ **New repo** under ovn-kubernetes organization
3. ✅ **OKEP first** before implementation

### Top Priorities (From Meeting)

1. 🔴 **VM Live Migration** (KubeVirt integration)
2. 🔴 **Non-overlapping CIDR coordination** (IPAM)
3. 🔴 **GRE transport for hardware offload**
4. 🟡 **CUDN type coverage** (Primary/Secondary, L2/L3)
5. 🟡 **Transport flexibility** (not just EVPN)

### Open Questions (Need to Answer in OKEP)

1. **Repository naming:** `ovn-mcn`, `multi-cluster-networking`, or other?
2. **Default network stretching:** In scope for future or never?
3. **Submariner integration:** Complementary or overlapping?
4. **IPAM pluggability:** Allow external IPAM systems?
5. **Live vs cold migration:** Which is primary use case?

---

## Recommended Work Plan

### Week 1: Foundation (May 12-16)

**Goal:** Understand CUDN types and design IPAM

**Tasks:**
- [ ] Read `OKEP_PLANNING.md` fully
- [ ] Review `OKEP_DRAFT.md`, add notes
- [ ] **Research Task 1:** CUDN Topology Analysis
  - Read OVN-K CUDN docs
  - Test layer2, layer3, localnet CUDNs locally
  - Document findings in `docs/CUDN_TOPOLOGY_ANALYSIS.md`
- [ ] **Research Task 7:** IPAM Controller Design
  - Review existing allocators (ASN, VNI, VTEP)
  - Design CIDR allocation algorithm
  - Draft `docs/IPAM_CONTROLLER_DESIGN.md`

**Deliverables:**
- `docs/CUDN_TOPOLOGY_ANALYSIS.md`
- `docs/IPAM_CONTROLLER_DESIGN.md`
- Updated `OKEP_DRAFT.md` with findings

---

### Week 2: Integration & Transport (May 19-23)

**Goal:** Understand KubeVirt integration and GRE transport

**Tasks:**
- [ ] **Research Task 3:** KubeVirt Migration API
  - Read KubeVirt docs
  - Set up test VM migration
  - Prototype MCN integration
  - Document in `docs/KUBEVIRT_INTEGRATION.md`
- [ ] **Research Task 2:** GRE Hardware Offload
  - Research NIC capabilities
  - Test GRE tunnels (if hardware available)
  - Benchmark EVPN vs GRE
  - Document in `docs/GRE_OFFLOAD_ANALYSIS.md`
- [ ] Update `OKEP_DRAFT.md` with KubeVirt and GRE sections

**Deliverables:**
- `docs/KUBEVIRT_INTEGRATION.md`
- `docs/GRE_OFFLOAD_ANALYSIS.md`
- Updated use cases in OKEP

---

### Week 3: Scale & Validation (May 26-30)

**Goal:** Design for scale and validate L2 support

**Tasks:**
- [ ] **Research Task 5:** Route Reflector Design
  - Review BGP RR best practices
  - Test RR in 5-cluster setup
  - Document in `docs/ROUTE_REFLECTOR_DESIGN.md`
- [ ] **Research Task 6:** EVPN Type-2 Validation
  - Create layer2 CUDN
  - Test L2 stretching
  - Document in `docs/EVPN_TYPE2_VALIDATION.md`
- [ ] **Research Task 4:** Managed VTEP Status (optional)
  - Check OVN-K roadmap
  - Document in `docs/MANAGED_VTEP_STATUS.md`

**Deliverables:**
- `docs/ROUTE_REFLECTOR_DESIGN.md`
- `docs/EVPN_TYPE2_VALIDATION.md`
- `docs/MANAGED_VTEP_STATUS.md`

---

### Week 4: Finalize & Submit (Jun 2-6)

**Goal:** Complete OKEP draft and submit for review

**Tasks:**
- [ ] Complete all research tasks
- [ ] Add diagrams to OKEP:
  - Architecture diagram (broker-agent)
  - API flow diagram (user → MCNC → MCN)
  - Transport plugin architecture
  - IPAM allocation flow
- [ ] Write implementation plan section
- [ ] Proofread and polish OKEP
- [ ] Submit to OVN-K community:
  - Post to mailing list
  - Share in Slack
  - Request review from maintainers

**Deliverables:**
- ✅ Complete OKEP draft
- ✅ All research documents
- ✅ Community submission

---

## Resources

### Documentation
- [OVN-K User Defined Networks](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/user-defined-networks.md)
- [Kubernetes KEP Template](https://github.com/kubernetes/enhancements/blob/master/keps/NNNN-kep-template/README.md)
- [Submariner Architecture](https://submariner.io/getting-started/architecture/)
- [KubeVirt Live Migration](https://kubevirt.io/user-guide/operations/live_migration/)

### Code
- [MCN PoC (vtep_unmanaged)](https://github.com/yboaron/mcn/tree/vtep_unmanaged)
- [OVN-K VTEP Controller](https://github.com/ovn-org/ovn-kubernetes/tree/master/go-controller/pkg/crd/vtep)
- [OVN-K CUDN Controller](https://github.com/ovn-org/ovn-kubernetes/tree/master/go-controller/pkg/controller/user-defined-network)

### Community
- **OVN-K Meetings:** Bi-weekly Thursdays
- **Slack:** #ovn-kubernetes
- **Mailing List:** ovn-kubernetes@googlegroups.com
- **Meeting Notes:** [Link to doc]

---

## Common Commands

### Read Documents
```bash
# Master plan
cat docs/OKEP_PLANNING.md

# OKEP draft
cat docs/OKEP_DRAFT.md

# Task tracker
cat docs/RESEARCH_TASKS.md
```

### Start Research Task
```bash
# Example: Task 1 (CUDN Topology Analysis)
cd docs/
touch CUDN_TOPOLOGY_ANALYSIS.md

# Open in editor and start documenting findings
```

### Update OKEP Draft
```bash
# Edit OKEP
vim docs/OKEP_DRAFT.md

# Add findings from research
# Fill in TODOs
# Add diagrams (using mermaid or ASCII art)
```

### Track Progress
```bash
# Update task status
vim docs/RESEARCH_TASKS.md

# Change status from "🔄 Not Started" to "🏃 In Progress"
# Add notes on progress
```

---

## Need Help?

### Stuck on Research Task?
- Review the "Methodology" section in `RESEARCH_TASKS.md`
- Check similar implementations (Calico, Cilium, Submariner)
- Ask on OVN-K Slack

### Unclear About Use Case?
- Review community meeting notes
- Check `OKEP_PLANNING.md` Section 3 (Use Cases)
- Ask stakeholders for clarification

### OKEP Structure Questions?
- Reference Kubernetes KEP template
- Look at approved OVN-K enhancement proposals (if any exist)
- Follow standard KEP format (Summary → Motivation → Proposal → Design → Risks)

---

## Success Criteria

### Week 1 Success
- ✅ Understand all CUDN topology types
- ✅ IPAM controller design drafted
- ✅ Can explain CUDN stretching feasibility

### Week 2 Success
- ✅ KubeVirt integration path clear
- ✅ GRE transport feasibility determined
- ✅ Use cases updated with research findings

### Week 3 Success
- ✅ Scale strategy designed (route reflector)
- ✅ L2 CUDN stretching validated
- ✅ All research tasks complete

### Week 4 Success
- ✅ OKEP draft complete and polished
- ✅ Diagrams added
- ✅ Submitted to community for review

---

**Good luck!** You have a clear plan, strong community support, and a working PoC to build from. The next 4 weeks are about formalizing the design and getting community alignment.

**Questions?** Review the planning documents or reach out to the OVN-K community.

---

**Document Created:** 2026-05-12  
**Owner:** Yossi Boaron  
**Next Review:** 2026-05-19
