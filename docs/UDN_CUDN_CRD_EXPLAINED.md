# UDN and CUDN CRDs Explained
**Understanding OVN-K Network CRDs vs MCN Enhancements**

---

## ⚠️ Important Distinction

**Standard OVN-K CRDs** (what exists in upstream OVN-K):
- `UserDefinedNetwork` (UDN) - Namespace-scoped
- `ClusterUserDefinedNetwork` (CUDN) - Cluster-scoped
- `RouteAdvertisement` - For BGP route advertisement

**MCN Enhancements** (what we ADD for multi-cluster):
- Patch CUDN with EVPN config (VNI, RouteTarget)
- Create RouteAdvertisement CRs
- Transport selection is in **MCN's MCNC CRD**, not CUDN!

**You won't find "transport" in CUDN** - that's a MCN concept!

---

## 📋 CUDN CRD Fields (Standard OVN-K)

### Full CUDN Spec

```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: my-network
spec:
  # ========== NETWORK CONFIGURATION ==========
  network:
    # CIDR allocation
    subnets:
      - "10.100.0.0/16"  # IPv4 subnet
      - "fd00:10:100::/64"  # IPv6 subnet (optional)
    
    # Network role
    role: secondary  # or "primary"
    
    # MTU (optional)
    mtu: 1400
  
  # ========== TOPOLOGY TYPE ==========
  topology: layer3  # or "layer2" or "localnet"
  
  # ========== LAYER 2 SPECIFIC CONFIG ==========
  # Only used if topology: layer2
  layer2:
    # IPAM config for layer2 networks
    ipamLifecycle: Persistent  # or Ephemeral
    
    # Subnets (if different from network.subnets)
    subnets:
      - cidr: "192.168.1.0/24"
        excludeSubnets:
          - "192.168.1.1/32"  # Gateway
        hostSubnet: "192.168.1.0/26"  # For node IPs
  
  # ========== LOCALNET SPECIFIC CONFIG ==========
  # Only used if topology: localnet
  localnetConfig:
    # External bridge name
    bridgeName: br-ex
    
    # VLAN tag (optional)
    vlanID: 100
    
    # MTU override
    mtu: 1500

  # ========== NAMESPACE SELECTOR ==========
  # Which namespaces can use this CUDN
  namespaceSelector:
    matchLabels:
      environment: production
    # or matchExpressions for more complex selection
```

---

## 🔍 Field-by-Field Explanation

### 1. `spec.network` - Network Configuration

**`spec.network.subnets`** (Required)
```yaml
subnets:
  - "10.100.0.0/16"  # IPv4
  - "fd00:10:100::/64"  # IPv6 (dual-stack)
```
- **What it is:** CIDR ranges for pod IPs on this network
- **IPv4, IPv6, or dual-stack:** You can have one or both
- **How MCN uses it:** MCN IPAM will coordinate these across clusters (allocate non-overlapping subnets)

---

**`spec.network.role`** (Optional, default: `secondary`)
```yaml
role: secondary  # or "primary"
```

**Values:**
- **`secondary`** (default):
  - Network attached as additional interface (net1, net2, etc.)
  - Uses Multus CNI
  - Pods must opt-in with annotation: `k8s.v1.cni.cncf.io/networks: my-network`
  - Pod interfaces:
    - `eth0` = Default cluster network
    - `net1` = This CUDN (secondary)
  
- **`primary`**:
  - Network used as primary pod interface (replaces default network)
  - Pod gets CUDN IP on `eth0`
  - **NOT RECOMMENDED for MCN** - security implications
  - Check if OVN-K even supports this (may be planned, not implemented)

**MCN Recommendation:** Support `secondary` only (safer, opt-in)

---

**`spec.network.mtu`** (Optional)
```yaml
mtu: 1400
```
- **What it is:** Maximum Transmission Unit for this network
- **Default:** Inherited from cluster default (usually 1400 for OVN)
- **Why it matters for MCN:** Cross-cloud may need lower MTU (VPN overhead)

---

### 2. `spec.topology` - How OVN Implements the Network

**Values:** `layer2`, `layer3`, `localnet`

#### **`topology: layer3`** ← Your PoC uses this!

```yaml
topology: layer3
```

**What OVN creates:**
- **OVN Logical Router** (LR) per CUDN
- Routes between nodes
- NAT if needed

**Behavior:**
- **L3 routed connectivity** - different broadcast domains
- Pods on different nodes are in different L2 segments
- No broadcast, no ARP between nodes
- Packets are routed via OVN LR

**Stretching with MCN:**
- ✅ **Easy** - Just advertise IP prefixes via BGP
- **Transport:** EVPN Type-5 routes (IP prefix advertisement)
- **GRE alternative:** L3 GRE tunnels

**Example OVN topology:**
```
Node1-Pod ──[L3]──> OVN-LR ──[L3]──> Node2-Pod
                      ↓
                  BGP EVPN (Type-5: "10.100.0.0/24 via 100.0.0.1")
                      ↓
              Remote Cluster OVN-LR
```

---

#### **`topology: layer2`**

```yaml
topology: layer2
```

**What OVN creates:**
- **OVN Logical Switch** (LS) per CUDN
- MAC learning table
- Broadcast domain

**Behavior:**
- **L2 switched connectivity** - same broadcast domain
- Pods can ARP for each other (even on different nodes)
- Broadcast/multicast works
- OVN handles L2 tunneling between nodes

**Stretching with MCN:**
- 🟡 **Medium complexity** - Need to advertise MAC/IP pairs
- **Transport:** EVPN Type-2 routes (MAC/IP advertisement)
- **GRE alternative:** L2 GRE tunnels
- **Challenge:** Broadcast/multicast handling across clusters

**Example OVN topology:**
```
Node1-Pod ──[L2]──> OVN-LS ──[L2]──> Node2-Pod
                      ↓ (ARP broadcast works)
                  BGP EVPN (Type-2: "MAC aa:bb:cc + IP 10.100.0.5 via 100.0.0.1")
                      ↓
              Remote Cluster OVN-LS
```

**When to use layer2:**
- VM migration (VM expects same L2 segment)
- Legacy apps expecting broadcast
- Clustering software using multicast
- Applications doing MAC-based discovery

---

#### **`topology: localnet`**

```yaml
topology: localnet
localnetConfig:
  bridgeName: br-ex  # External bridge
  vlanID: 100        # VLAN tag (optional)
```

**What OVN creates:**
- **Localnet port** connecting OVN to external bridge
- Direct connection to physical network

**Behavior:**
- Pods connect to external/physical network
- No OVN tunneling (direct bridge connection)
- Requires external bridge (br-ex) on all nodes
- Often used for provider networks

**Stretching with MCN:**
- ❌ **Very hard / out of scope**
- Requires external bridge on all clusters
- Physical network dependency
- Probably not supported in Phase 1

**When to use localnet:**
- Connect pods to existing physical network
- Provider networks in cloud environments
- Direct access to VLANs

---

### 3. `spec.layer2` - Layer2-Specific Config

**Only used when `topology: layer2`**

```yaml
topology: layer2
layer2:
  # IPAM lifecycle
  ipamLifecycle: Persistent  # or Ephemeral
  
  # Detailed subnet config
  subnets:
    - cidr: "192.168.1.0/24"
      excludeSubnets:
        - "192.168.1.1/32"     # Reserve for gateway
        - "192.168.1.2/32"     # Reserve for DHCP
      hostSubnet: "192.168.1.0/26"  # IPs for node interfaces
```

**`ipamLifecycle`:**
- **`Persistent`**: IP addresses are stable (pod keeps same IP on restart)
- **`Ephemeral`**: IP addresses are dynamic (new IP on restart)

**Subnet exclusions:**
- Reserve IPs for gateways, DHCP servers, etc.
- `hostSubnet`: Range used for node-facing IPs

---

### 4. `spec.localnetConfig` - Localnet-Specific Config

**Only used when `topology: localnet`**

```yaml
topology: localnet
localnetConfig:
  bridgeName: br-ex  # Required: external bridge name
  vlanID: 100        # Optional: VLAN tag
  mtu: 1500          # Optional: MTU override
```

**`bridgeName`:**
- Name of Linux bridge on nodes (e.g., `br-ex`, `br-provider`)
- Must exist on all nodes
- OVN creates localnet port connected to this bridge

**`vlanID`:**
- VLAN tag for traffic on external network
- If specified, OVN tags packets with this VLAN ID

---

### 5. `spec.namespaceSelector` - Which Namespaces Can Use This CUDN

```yaml
namespaceSelector:
  matchLabels:
    environment: production
  matchExpressions:
    - key: team
      operator: In
      values: ["platform", "infra"]
```

- Controls which namespaces can attach pods to this CUDN
- Label-based selection
- If not specified, all namespaces can use it

---

## 🔧 Where Does EVPN Config Come From?

**Answer: MCN adds it!** CUDN itself has no EVPN fields.

### Standard CUDN (OVN-K)
```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
spec:
  network:
    subnets: ["10.100.0.0/16"]
    role: secondary
  topology: layer3
# ← No EVPN config here! Just network definition.
```

### MCN Patches CUDN with EVPN Annotations

When MCN stretches a CUDN, it **patches** the CUDN with EVPN config:

```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
  annotations:
    # MCN adds these annotations
    mcn.ovn.org/vni: "5001"
    mcn.ovn.org/route-target: "65000:5001"
spec:
  network:
    subnets: ["10.100.0.0/16"]  # May be adjusted by MCN IPAM
    role: secondary
  topology: layer3
```

### MCN Creates RouteAdvertisement CR

In addition to patching CUDN, MCN creates a **RouteAdvertisement** CR:

```yaml
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisement
metadata:
  name: blue-network-evpn
spec:
  # Network to advertise
  networkSelector:
    matchLabels:
      name: blue-network
  
  # BGP configuration
  targetVRF: "evpn"
  
  # EVPN parameters (THIS is where EVPN config goes!)
  evpn:
    vni: 5001
    routeTarget: "65000:5001"
    routeDistinguisher: "65000:5001"
  
  # Advertise these subnets
  advertiseSubnets:
    - "10.100.0.0/17"  # This cluster's allocated subnet
```

**RouteAdvertisement** tells OVN-K's EVPN controller to:
- Advertise routes for this CUDN via BGP
- Use specified VNI and Route Target
- Export routes to FRR for BGP peering

---

## 🌐 MCN CRDs - Where Transport Selection Happens

MCN introduces **new CRDs** for multi-cluster orchestration:

### MultiClusterNetworkConnect (MCNC) - User Intent to Stretch CUDN

**This is where user specifies transport!**

```yaml
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  # Which MCN to join
  multiClusterNetwork: blue-network
  
  # Which local CUDN to stretch
  localNetwork: blue-network
  
  # ← TRANSPORT IS HERE! MCN-specific, not in CUDN
  transport: evpn  # or "gre", "geneve", etc.
  
  # Transport-specific config
  transportConfig:
    # EVPN-specific (if transport: evpn)
    evpn:
      vni: 5001  # Can be auto-allocated by MCN
      routeTarget: "65000:5001"
    
    # GRE-specific (if transport: gre)
    gre:
      ttl: 64
      offload: true
```

**What happens when you create MCNC:**
1. MCN agent sees MCNC CR
2. Agent syncs with broker, gets VNI allocation
3. Agent **patches CUDN** with EVPN annotations
4. Agent **creates RouteAdvertisement** CR with VNI/RT
5. OVN-K EVPN controller sees RouteAdvertisement
6. OVN-K advertises routes to FRR
7. FRR establishes BGP peering with remote clusters

---

## 📊 Complete CRD Hierarchy

```
┌─────────────────────────────────────────────────────────┐
│  CUDN (OVN-K standard)                                  │
│  - Network definition (subnets, topology, role)         │
│  - NO transport or EVPN config                          │
└─────────────────────────────────────────────────────────┘
                        │
                        │ User creates MCNC to stretch it
                        ▼
┌─────────────────────────────────────────────────────────┐
│  MultiClusterNetworkConnect (MCN CRD)                   │
│  - User intent: stretch blue-network across clusters    │
│  - Transport selection: evpn, gre, etc.                 │
│  - Transport config: VNI, RT, GRE options               │
└─────────────────────────────────────────────────────────┘
                        │
                        │ MCN agent processes MCNC
                        ▼
┌─────────────────────────────────────────────────────────┐
│  CUDN (patched by MCN)                                  │
│  - Annotations added: mcn.ovn.org/vni, route-target     │
│  - Subnets adjusted by MCN IPAM                         │
└─────────────────────────────────────────────────────────┘
                        +
┌─────────────────────────────────────────────────────────┐
│  RouteAdvertisement (created by MCN)                    │
│  - EVPN config: VNI, Route Target, Route Distinguisher  │
│  - Tells OVN-K to advertise routes via BGP              │
└─────────────────────────────────────────────────────────┘
                        │
                        │ OVN-K processes RouteAdvertisement
                        ▼
┌─────────────────────────────────────────────────────────┐
│  FRRConfiguration (created by OVN-K or MCN)             │
│  - BGP peering config                                   │
│  - BGP neighbors (remote cluster VTEPs)                 │
│  - Address families (l2vpn evpn)                        │
└─────────────────────────────────────────────────────────┘
```

---

## 🔍 Real-World Example

### Step 1: User Creates CUDN (Standard OVN-K)

```yaml
# User creates CUDN on cluster1
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
spec:
  network:
    subnets: ["10.100.0.0/16"]  # User wants this range
    role: secondary
  topology: layer3
```

**Result:** Local layer3 network created, no multi-cluster yet.

---

### Step 2: User Stretches CUDN (MCN MCNC)

```yaml
# User creates MCNC on cluster1 to stretch CUDN
apiVersion: multicluster.ovn.org/v1
kind: MultiClusterNetworkConnect
metadata:
  name: blue-network-stretch
spec:
  multiClusterNetwork: blue-network
  localNetwork: blue-network
  transport: evpn  # ← User selects transport here!
```

**Result:** MCN agent starts orchestration.

---

### Step 3: MCN Agent Allocates Resources

MCN agent talks to broker:
1. Checks if MultiClusterNetwork "blue-network" exists on broker
2. If not, creates MCN with VNI allocation (e.g., VNI 5001)
3. Requests CIDR allocation from broker IPAM
4. Gets allocated subnet: `10.100.0.0/17` (cluster1's portion)

---

### Step 4: MCN Agent Patches CUDN

```yaml
# MCN patches CUDN with EVPN config
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue-network
  annotations:
    mcn.ovn.org/vni: "5001"  # ← MCN added
    mcn.ovn.org/route-target: "65000:5001"  # ← MCN added
spec:
  network:
    subnets: ["10.100.0.0/17"]  # ← MCN adjusted via IPAM
    role: secondary
  topology: layer3
```

---

### Step 5: MCN Agent Creates RouteAdvertisement

```yaml
# MCN creates RouteAdvertisement
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisement
metadata:
  name: blue-network-evpn
spec:
  networkSelector:
    matchLabels:
      name: blue-network
  targetVRF: "evpn"
  evpn:
    vni: 5001
    routeTarget: "65000:5001"
  advertiseSubnets:
    - "10.100.0.0/17"
```

---

### Step 6: OVN-K Advertises Routes

OVN-K EVPN controller:
1. Sees RouteAdvertisement CR
2. Programs OVN with EVPN config
3. Advertises routes to FRR:
   ```
   BGP EVPN Type-5 route:
   - VNI: 5001
   - Prefix: 10.100.0.0/17
   - Next-hop: 100.0.0.1 (cluster1 VTEP)
   - Route Target: 65000:5001
   ```

---

### Step 7: FRR Propagates Routes

FRR on cluster1:
- Receives route from OVN-K
- Advertises to BGP neighbors (cluster2)
- cluster2 FRR receives route
- cluster2 installs route: `10.100.0.0/17 via 100.0.0.1`

**Result:** Cross-cluster connectivity achieved! 🎉

---

## 📝 Summary: Key Takeaways

### CUDN CRD (OVN-K Standard)
- ✅ Has: `network.subnets`, `network.role`, `topology`
- ❌ Does NOT have: `transport`, `vni`, `routeTarget`
- Purpose: Define local network on single cluster

### MCN Enhancements
- **MCNC CRD:** User intent to stretch + transport selection
- **Patches CUDN:** Add EVPN annotations (VNI, RT)
- **Creates RouteAdvertisement:** Tell OVN-K to advertise via BGP
- **Creates FRRConfiguration:** BGP peering setup

### Transport Selection
- **Not in CUDN!** It's in MCN's MCNC CRD
- User specifies: `spec.transport: evpn` or `gre` or others
- MCN agent implements transport-specific logic

---

## 🚀 Next Steps for Investigation

Now that you understand CUDN fields:

1. **Read OVN-K CUDN docs** with this knowledge
   - Focus on `topology`, `network.role`, `network.subnets`
   - Ignore missing EVPN fields (those are MCN additions)

2. **Test different topologies:**
   ```bash
   # Try layer3 (easy, already working)
   # Try layer2 (check if EVPN Type-2 works)
   # Try localnet (probably won't work in KIND)
   ```

3. **Document findings:**
   - Which topologies work locally?
   - Which can be stretched across clusters?
   - What transport does each need? (EVPN Type-2 vs Type-5)

4. **Update OKEP:**
   - Add CUDN field reference section
   - Clarify what's OVN-K vs what's MCN
   - Show how MCNC adds transport on top of CUDN

---

## 📞 Questions for OVN-K Community

Since you're investigating, good questions to ask:

1. **"Does OVN-K support `role: primary` for CUDNs?"**
   - Check if primary is implemented or planned
   - Security implications?

2. **"Does OVN-K EVPN controller support Type-2 routes for layer2 topologies?"**
   - Critical for L2 stretching
   - VM migration use case

3. **"What's the intended use case for localnet topology?"**
   - Is it meant for provider networks?
   - How does it work with EVPN?

---

**Hope this clears up the confusion!** CUDN is just the network definition. MCN adds the multi-cluster magic on top. 🚀
