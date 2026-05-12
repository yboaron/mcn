# CUDN Types & Topologies Learning Guide
**Day 1-4 Investigation Resource**

---

## Quick Overview: What You're Learning

**CUDN = ClusterUserDefinedNetwork** (OVN-K's way to create custom pod networks)

You need to understand **3 dimensions** of CUDNs:

1. **Topology Type** (spec.topology): How OVN implements the network
   - `layer2` - L2 switching (OVN logical switch)
   - `layer3` - L3 routing (OVN logical router)
   - `localnet` - Bridge to external network

2. **Network Type** (L2 vs L3 behavior): How packets are forwarded
   - **L2** - Same broadcast domain, ARP works, MAC learning
   - **L3** - Routed, different broadcast domains, IP forwarding

3. **Role Type** (spec.network.role): How it's attached to pods
   - `secondary` - Additional network via Multus (standard)
   - `primary` - Can it be the main pod network? (research needed)

---

## 📚 Essential Documentation

### OVN-K Official Docs

**1. User Defined Networks Overview**
```
https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/user-defined-networks.md
```
**What to read:**
- Introduction section (what are UDNs?)
- Cluster vs Namespace scoped UDNs
- Network topology types
- Examples at the bottom

**Key sections:**
- "Network Topology" - Explains layer2, layer3, localnet
- "Primary vs Secondary" - Role types
- YAML examples for each topology

---

**2. User Defined Networks Design Doc**
```
https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/design/user-defined-networks.md
```
**What to read:**
- Architecture overview
- OVN implementation details (logical switches vs routers)
- How each topology is implemented in OVN

**Key sections:**
- "Topology Implementation" - How OVN creates LS/LR for each type
- "Multus Integration" - How secondary networks work
- "Port Binding" - How pods connect to UDNs

---

**3. CUDN CRD Definition (Code)**
```
https://github.com/ovn-org/ovn-kubernetes/blob/master/go-controller/pkg/crd/userdefinednetwork/v1/types.go
```
**What to read:**
- `ClusterUserDefinedNetwork` struct
- `spec.topology` field - Valid values and meanings
- `spec.network.role` field - Primary vs Secondary
- `spec.network.subnets` - CIDR allocation

**Key structs to understand:**
```go
type ClusterUserDefinedNetwork struct {
    Spec ClusterUserDefinedNetworkSpec
}

type ClusterUserDefinedNetworkSpec struct {
    Topology string  // "layer2", "layer3", "localnet"
    Network  NetworkSpec
}

type NetworkSpec struct {
    Role    string    // "primary", "secondary"
    Subnets []string  // ["10.100.0.0/16"]
}
```

---

### Multus Documentation (for Secondary Networks)

**Multus CNI Overview**
```
https://github.com/k8snetworkplumbingwg/multus-cni
```
**What to read:**
- How Multus attaches additional networks to pods
- `k8s.v1.cni.cncf.io/networks` annotation
- Network attachment definitions

**Key concept:** Secondary networks use Multus to attach as `net1`, `net2` etc., while `eth0` remains the default network.

---

## 🔍 Code Locations to Explore

### 1. CUDN Controller
```bash
# Clone OVN-K if you don't have it locally
git clone https://github.com/ovn-org/ovn-kubernetes.git
cd ovn-kubernetes

# Find CUDN controller code
find go-controller -name "*user-defined-network*" -type f

# Key files:
# go-controller/pkg/controller/user-defined-network/controller.go
# go-controller/pkg/controller/user-defined-network/layer2_controller.go
# go-controller/pkg/controller/user-defined-network/layer3_controller.go
```

**What to look for in controller code:**
- How `layer2` CUDNs create OVN logical switches
- How `layer3` CUDNs create OVN logical routers
- How `localnet` CUDNs connect to external bridges
- Port creation logic for pods

---

### 2. OVN Logical Topology Creation

**File:** `go-controller/pkg/controller/user-defined-network/layer3_controller.go`

**Key function:** `setupClusterNetwork()` or similar
- Creates OVN logical router for layer3 topology
- Sets up routing between nodes
- Configures NAT if needed

**File:** `go-controller/pkg/controller/user-defined-network/layer2_controller.go`

**Key function:** `setupLayer2Network()` or similar
- Creates OVN logical switch for layer2 topology
- Enables broadcast/multicast if needed
- Handles ARP responder

---

### 3. EVPN Integration (Important for MCN!)

**File:** `go-controller/pkg/crd/evpn/` directory
- How OVN-K integrates with EVPN
- RouteAdvertisement handling
- EVPN Type-2 (MAC/IP) vs Type-5 (IP prefix) routes

---

## 🛠️ Hands-On Commands

### Setup: Use Your Existing Test Clusters

You already have 2 clusters from vtep_unmanaged deployment:
```bash
# Check cluster status
kind get clusters
# Output: cluster1, cluster2

# Get kubeconfigs
export KUBECONFIG1=/home/yboaron/prj/mcn/output/kubeconfig-cluster1.yaml
export KUBECONFIG2=/home/yboaron/prj/mcn/output/kubeconfig-cluster2.yaml

# Verify clusters are ready
kubectl --kubeconfig=$KUBECONFIG1 get nodes
kubectl --kubeconfig=$KUBECONFIG2 get nodes
```

---

### Test 1: Create and Inspect layer3 CUDN (L3 Routing)

**layer3 = L3 routing, different broadcast domains**

```bash
# Create layer3 CUDN
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-layer3
spec:
  network:
    subnets: ["10.200.0.0/16"]
    role: secondary  # ← Secondary network (via Multus)
  topology: layer3   # ← L3 routing
EOF

# Wait for CUDN to be ready
kubectl --kubeconfig=$KUBECONFIG1 get cudn test-layer3 -w

# Check OVN logical routers created
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl lr-list

# Expected output: Should see a logical router for test-layer3
# Example: "lr-test-layer3" or similar name

# Check logical router details
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl lr-nat-list <router-name>

# Check routes
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl lr-route-list <router-name>
```

**Deploy pod on layer3 network:**
```bash
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: pod-layer3-1
  annotations:
    k8s.v1.cni.cncf.io/networks: test-layer3  # ← Attach to secondary network
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

# Wait for pod to be ready
kubectl --kubeconfig=$KUBECONFIG1 wait --for=condition=ready pod/pod-layer3-1 --timeout=60s

# Check pod interfaces
kubectl --kubeconfig=$KUBECONFIG1 exec pod-layer3-1 -- ip addr

# Expected output:
# - eth0: Default network (e.g., 10.244.x.x)
# - net1: layer3 network (e.g., 10.200.0.5) ← Secondary network from test-layer3
```

**What you're learning:**
- ✅ layer3 topology creates OVN logical router
- ✅ Secondary network attached as `net1` via Multus
- ✅ Pod gets IP from CUDN subnet (10.200.0.0/16)
- ✅ L3 routing (not same broadcast domain as other pods)

---

### Test 2: Create and Inspect layer2 CUDN (L2 Switching)

**layer2 = L2 switching, same broadcast domain, ARP works across nodes**

```bash
# Create layer2 CUDN
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-layer2
spec:
  network:
    subnets: ["192.168.100.0/24"]
    role: secondary
  topology: layer2   # ← L2 switching
EOF

# Check OVN logical switches created
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl ls-list

# Expected output: Should see a logical switch for test-layer2
# Example: "ls-test-layer2" or similar

# Check logical switch ports
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl lsp-list <switch-name>

# Check MAC learning
kubectl --kubeconfig=$KUBECONFIG1 exec -n ovn-kubernetes \
  $(kubectl --kubeconfig=$KUBECONFIG1 get pods -n ovn-kubernetes -l name=ovnkube-node -o jsonpath='{.items[0].metadata.name}') \
  -- ovn-nbctl list Logical_Switch <switch-name>
```

**Deploy pods on layer2 network:**
```bash
# Pod 1
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: pod-layer2-1
  annotations:
    k8s.v1.cni.cncf.io/networks: test-layer2
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

# Pod 2 on different node (if available)
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: pod-layer2-2
  annotations:
    k8s.v1.cni.cncf.io/networks: test-layer2
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
  nodeSelector:
    kubernetes.io/hostname: cluster1-worker  # Force to worker node
EOF

# Wait for pods
kubectl --kubeconfig=$KUBECONFIG1 wait --for=condition=ready pod/pod-layer2-1 --timeout=60s
kubectl --kubeconfig=$KUBECONFIG1 wait --for=condition=ready pod/pod-layer2-2 --timeout=60s

# Test L2 connectivity (ARP, broadcast)
# Get pod2's IP on layer2 network
POD2_IP=$(kubectl --kubeconfig=$KUBECONFIG1 exec pod-layer2-2 -- ip -4 addr show net1 | grep -oP '(?<=inet\s)\d+(\.\d+){3}')

# Test ARP from pod1 to pod2
kubectl --kubeconfig=$KUBECONFIG1 exec pod-layer2-1 -- arping -c 3 -I net1 $POD2_IP

# Check ARP table on pod1
kubectl --kubeconfig=$KUBECONFIG1 exec pod-layer2-1 -- ip neigh show dev net1

# Ping to verify connectivity
kubectl --kubeconfig=$KUBECONFIG1 exec pod-layer2-1 -- ping -c 3 $POD2_IP
```

**What you're learning:**
- ✅ layer2 topology creates OVN logical switch
- ✅ Pods in same L2 domain can ARP for each other
- ✅ Broadcast domain spans across nodes (OVN tunnels handle this)
- ✅ MAC learning happens in OVN logical switch

**Key difference from layer3:**
- **layer2:** Pods can ARP → same broadcast domain
- **layer3:** Pods cannot ARP → routed, different broadcast domains

---

### Test 3: Investigate localnet CUDN (External Bridge)

**localnet = Bridge to external/physical network**

```bash
# Research: What is localnet topology?
# From OVN-K docs: Connects to external network via bridge

# Check if your clusters support localnet
kubectl --kubeconfig=$KUBECONFIG1 get nodes -o wide

# localnet requires:
# 1. External bridge on node (e.g., br-ex)
# 2. Bridge connected to physical network
# 3. VLAN tagging if needed

# Try creating localnet CUDN (may fail if no external bridge)
kubectl --kubeconfig=$KUBECONFIG1 apply -f - <<EOF
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: test-localnet
spec:
  network:
    subnets: ["172.16.0.0/24"]
    role: secondary
  topology: localnet
  localnetConfig:
    bridgeName: br-ex  # External bridge name
    vlanID: 100        # VLAN tag (optional)
EOF

# Check if CUDN was created
kubectl --kubeconfig=$KUBECONFIG1 get cudn test-localnet

# Check error messages if it failed
kubectl --kubeconfig=$KUBECONFIG1 describe cudn test-localnet

# Expected: May fail if br-ex doesn't exist on nodes
# Document findings: Is localnet supported in KIND? Probably not.
```

**What you're learning:**
- ✅ localnet requires external bridge (br-ex or similar)
- ✅ Used for connecting pods to physical networks
- ✅ Probably NOT supported in KIND clusters (no external bridge)
- ✅ Recommendation: localnet stretching likely out of scope for MCN Phase 1

---

## 🧪 Understanding L2 vs L3 Behavior

### L2 Network Characteristics (layer2 topology)

**Same Broadcast Domain:**
```bash
# On layer2 network, broadcast reaches all pods
kubectl exec pod-layer2-1 -- ping -b -c 1 192.168.100.255

# ARP works across nodes
kubectl exec pod-layer2-1 -- arping -I net1 <other-pod-ip>

# MAC addresses are visible
kubectl exec pod-layer2-1 -- ip neigh show dev net1
```

**Use cases for L2:**
- VM migration (VM expects same L2 segment)
- Legacy applications expecting broadcast
- Clustering software using multicast
- Applications doing ARP for discovery

**Challenges for L2 stretching:**
- EVPN Type-2 routes needed (MAC/IP advertisement)
- Broadcast/multicast handling across clusters
- More complex than L3 stretching

---

### L3 Network Characteristics (layer3 topology)

**Routed Connectivity:**
```bash
# On layer3 network, no broadcast
kubectl exec pod-layer3-1 -- ping -b -c 1 10.200.255.255
# Expected: Fails or no response

# ARP doesn't work for pods on different nodes
kubectl exec pod-layer3-1 -- arping -I net1 <other-pod-ip>
# Expected: No ARP response (routed, not bridged)

# But IP connectivity works (routed via OVN LR)
kubectl exec pod-layer3-1 -- ping -c 3 <other-pod-ip>
# Expected: Works (routed)
```

**Use cases for L3:**
- Most modern microservices (HTTP/gRPC)
- Better scalability (no broadcast domain limits)
- Simpler to stretch across clusters
- Your PoC already uses this!

**Advantages for stretching:**
- EVPN Type-5 routes (IP prefixes only)
- No broadcast/multicast complexity
- Easier to implement, better performance

---

## 📊 Primary vs Secondary Networks

### Secondary Networks (Standard for CUDNs)

**How it works:**
```yaml
# CUDN with role: secondary (default)
spec:
  network:
    role: secondary  # ← Attached via Multus as net1, net2, etc.
```

**Pod attachment:**
```yaml
# Pod with secondary network
metadata:
  annotations:
    k8s.v1.cni.cncf.io/networks: test-layer3  # ← Multus attaches CUDN as net1
spec:
  containers:
  - name: app
    image: nginx
```

**Result:**
- `eth0` = Default pod network (e.g., 10.244.0.0/16)
- `net1` = Secondary network from CUDN (e.g., 10.200.0.0/16)

**Security:**
- ✅ Default network isolated from secondary networks
- ✅ Secondary networks are opt-in per pod
- ✅ Pods without annotation don't see secondary network

---

### Primary Networks (Research Needed)

**Question:** Can CUDN be a primary network (replace default pod network)?

**To investigate:**
```yaml
# Try creating CUDN with role: primary
spec:
  network:
    role: primary  # ← Primary network?
```

**Questions to answer:**
1. Does OVN-K support `role: primary` for CUDNs?
2. If yes, does pod get CUDN IP on `eth0` instead of default network?
3. What are security implications?
   - All pods on same primary network can communicate
   - Stretching primary network = cross-cluster default network
   - NetworkPolicy enforcement?

**Security concerns for stretching primary networks:**
- If primary network stretched, pods in different clusters share network
- Similar to stretching default pod network (risky!)
- Recommendation: **Support secondary networks only initially**

**Check OVN-K code:**
```bash
# Search for "primary" in CUDN controller
cd ovn-kubernetes
grep -r "role.*primary" go-controller/pkg/controller/user-defined-network/

# Check if primary is implemented or just secondary
```

---

## 🎯 Key Concepts Summary

### Topology Type (How OVN Implements It)

| Topology | OVN Object | Behavior | Broadcast | Stretching |
|----------|-----------|----------|-----------|------------|
| **layer3** | Logical Router | L3 routing | No | ✅ Easy (EVPN Type-5) |
| **layer2** | Logical Switch | L2 switching | Yes | 🟡 Medium (EVPN Type-2) |
| **localnet** | External Bridge | Physical network | Yes | ❌ Hard (out of scope) |

### Network Type (L2 vs L3 Behavior)

| Type | Broadcast | ARP | Use Cases | MCN Support |
|------|-----------|-----|-----------|-------------|
| **L3** | No | No | Modern apps, microservices | ✅ Phase 1 (PoC working) |
| **L2** | Yes | Yes | VMs, legacy apps, clustering | 🟡 Phase 2 (need EVPN Type-2) |

### Role Type (How Attached to Pods)

| Role | Interface | Default | Security | MCN Support |
|------|-----------|---------|----------|-------------|
| **secondary** | net1, net2 | eth0 = default | ✅ Isolated, opt-in | ✅ Phase 1 |
| **primary** | eth0 | None (CUDN is primary) | ⚠️ All pods same network | ❓ Research needed |

---

## 📝 Documentation Template

As you investigate, fill in this template in `CUDN_TOPOLOGY_ANALYSIS.md`:

```markdown
# CUDN Topology Analysis

## layer3 Topology
- **OVN Implementation:** Logical Router
- **Broadcast Domain:** No (routed)
- **ARP Support:** No (routed)
- **Use Cases:** Modern microservices, HTTP/gRPC apps
- **Stretching Feasibility:** ✅ Easy
- **Transport Requirements:** EVPN Type-5 (IP prefix routes), GRE L3
- **Phase 1 Support:** ✅ Yes (already working in PoC)

## layer2 Topology
- **OVN Implementation:** Logical Switch
- **Broadcast Domain:** Yes (same L2 segment)
- **ARP Support:** Yes
- **Use Cases:** VM migration, legacy apps, clustering
- **Stretching Feasibility:** 🟡 Medium complexity
- **Transport Requirements:** EVPN Type-2 (MAC/IP routes), GRE L2, broadcast handling
- **Phase 1 Support:** ❓ Need to validate EVPN Type-2 in OVN-K

## localnet Topology
- **OVN Implementation:** External bridge connection
- **Broadcast Domain:** Yes (physical network)
- **Use Cases:** Connect pods to physical network
- **Stretching Feasibility:** ❌ Out of scope (requires external bridge on all clusters)
- **Phase 1 Support:** ❌ No
```

---

## 🚀 Next Steps After Investigation

Once you complete Days 1-4:

1. **Document findings** in:
   - `CUDN_TOPOLOGY_ANALYSIS.md` (topology types)
   - `CUDN_TYPES_ANALYSIS.md` (L2/L3, Primary/Secondary)

2. **Update OKEP** sections:
   - "Design Details" → Add topology support matrix
   - "Use Cases" → Map use cases to topology types
   - "Graduation Criteria" → Phase 1 (layer3), Phase 2 (layer2)

3. **Determine MCN support:**
   - Phase 1: layer3 topology, secondary networks, L3 routing ✅
   - Phase 2: layer2 topology, EVPN Type-2 validation 🟡
   - Out of scope: localnet, primary networks ❌

---

## 🤔 Questions to Answer by Day 4

### Topology Questions
- [ ] Which topologies can be stretched? (layer3 ✅, layer2 ❓, localnet ❌)
- [ ] What OVN objects does each create? (LR vs LS vs external bridge)
- [ ] What transport does each need? (EVPN Type-5 vs Type-2, GRE L3 vs L2)

### Network Type Questions
- [ ] When to use L2 vs L3? (VM migration vs microservices)
- [ ] How does broadcast work in L2? (OVN tunnels)
- [ ] Can we stretch both L2 and L3? (L3 ✅, L2 needs validation)

### Role Type Questions
- [ ] Does OVN-K support primary networks? (Check code)
- [ ] Are there security implications for stretching primary? (All pods same network)
- [ ] Recommendation: Support secondary only? (Safer, opt-in)

### Stretching Feasibility
- [ ] Which combinations work?
  - layer3 + secondary + L3 routing ✅ (PoC working)
  - layer2 + secondary + L2 switching ❓ (Need EVPN Type-2 validation)
  - localnet + any ❌ (External bridge dependency)

---

## 📞 Need Help?

**OVN-K Community:**
- Slack: `#ovn-kubernetes`
- Mailing list: ovn-kubernetes@googlegroups.com
- Community meetings: Bi-weekly Thursdays

**Questions to ask:**
- "Does OVN-K support EVPN Type-2 routes for layer2 CUDNs?"
- "Can CUDNs be primary networks, or only secondary?"
- "What's the intended use case for localnet topology?"

---

**Ready to start?** Begin with reading the OVN-K user-defined-networks.md doc, then run the hands-on tests above!

**Good luck!** 🚀
