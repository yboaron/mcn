# FRR-K8s EVPN Integration Plan

## Overview

This document tracks the integration of FRR-K8s EVPN API support (PR #419) into SkyNet's BGP configurator.

**FRR-K8s EVPN PR**: https://github.com/metallb/frr-k8s/pull/419
**Status**: Open (not yet merged)

---

## Changes Made to SkyNet Code

### 1. Added `addressFamilies` Field to BGP Neighbors ✓

**File**: `pkg/agent/bgp/bgp_configurator.go`
**Function**: `buildNeighbors()`

```go
neighbor := map[string]interface{}{
    "address":         endpoint.BgpPeerIP,
    "asn":             remoteCluster.Spec.ASN,
    "ebgpMultiHop":    true,
    "addressFamilies": []string{"unicast", "evpn"},  // NEW
}
```

**What it does**: Activates both IPv4/IPv6 unicast and L2VPN EVPN address families on BGP neighbors.

**Why**: Required for EVPN neighbor activation according to new FRR-K8s API.

---

### 2. Added EVPN Configuration Builder ✓

**File**: `pkg/agent/bgp/bgp_configurator.go`
**Function**: `buildEVPNConfiguration()`

```go
func (c *BGPConfigurator) buildEVPNConfiguration(multiClusterNetworks []*skynetv1.MultiClusterNetwork) map[string]interface{} {
    evpnConfig := map[string]interface{}{
        "advertiseVNIs": "All",
    }

    var l2vnis []map[string]interface{}
    for _, mcn := range multiClusterNetworks {
        l2vni := map[string]interface{}{
            "vni":       mcn.Spec.VNI,                      // e.g., 5000
            "rd":        fmt.Sprintf("65000:%d", mcn.Spec.VNI),  // "65000:5000"
            "importRTs": []string{mcn.Spec.RouteTarget},    // ["65000:5000"]
            "exportRTs": []string{mcn.Spec.RouteTarget},    // ["65000:5000"]
        }
        l2vnis = append(l2vnis, l2vni)
    }

    evpnConfig["l2vnis"] = l2vnis
    return evpnConfig
}
```

**What it does**: Generates L2VNI EVPN configuration for each stretched MultiClusterNetwork.

**When it runs**: Only when `multiClusterNetworks` array is non-empty (i.e., when we have stretched networks).

---

### 3. Integrated EVPN Config into FRRConfiguration ✓

**File**: `pkg/agent/bgp/bgp_configurator.go`
**Function**: `buildFRRConfiguration()`

```go
// Add EVPN configuration if we have MultiClusterNetworks to advertise
if len(multiClusterNetworks) > 0 {
    router["evpn"] = c.buildEVPNConfiguration(multiClusterNetworks)
}
```

**What it does**: Adds EVPN section to BGP router when stretched networks exist.

---

## Example Generated FRRConfiguration

**Without stretched networks** (current Phase 1 - BGP peering only):

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: skynet-bgp-config
  namespace: frr-k8s-system
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: 172.18.0.5
        asn: 64513
        ebgpMultiHop: true
        addressFamilies: ["unicast", "evpn"]  # Ready for EVPN
```

**With stretched networks** (future Phase 2):

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: skynet-bgp-config
  namespace: frr-k8s-system
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: 172.18.0.5
        asn: 64513
        ebgpMultiHop: true
        addressFamilies: ["unicast", "evpn"]
      evpn:
        advertiseVNIs: All
        l2vnis:
        - vni: 5000
          rd: "65000:5000"
          importRTs: ["65000:5000"]
          exportRTs: ["65000:5000"]
```

---

## Deployment Updates

### FRR-K8s Installation ✓ (Via OVN-K with Route-Adv Cleanup)

**File**: `test/setup-clusters-v2.sh`

**Approach**: Use OVN-K PR #6127's `-rae` flag, then clean up route advertisement config:

```bash
# 1. OVN-K Configuration - EVPN PR branch with FRR-K8s
OVNK_REPO="https://github.com/jcaamano/ovn-kubernetes.git"
OVNK_BRANCH="evpn-frr-k8s-api"

# 2. Create clusters with -mne -rae flags
"$KIND_SH" -wk "$NUM_WORKERS" -ic -mne -rae
# -mne: multi-network enable (for CUDN support)
# -rae: route-advertisements enable (installs FRR-K8s with EVPN)

# 3. Clean up OVN-K's route advertisement FRRConfiguration
# (conflicts with SkyNet's per-cluster ASN)
kubectl delete frrconfiguration -n frr-k8s-system --all
```

**What happens**:
1. `-rae` flag triggers `install_frr_k8s()` in kind-common.sh
2. Clones jcaamano/frr-k8s evpn-plan branch (EVPN API support)
3. Builds FRR-K8s from source with EVPN CRDs
4. Deploys to cluster
5. **Problem**: Also creates route-adv FRRConfiguration with ASN 64512
6. **Solution**: Delete route-adv configs → clean FRR-K8s for SkyNet

**Benefits**:
- ✅ FRR-K8s installed via OVN-K's tested workflow
- ✅ EVPN API support from jcaamano/frr-k8s
- ✅ No ASN conflicts with SkyNet BGP
- ✅ Single `make deploy` command

**When PRs merge**: Update to upstream branches.

---

## Remaining Work

### Before Testing

- [x] **Update FRR-K8s deployment** to use EVPN-enabled version ✓
- [ ] **Test with current setup** - can test immediately
- [ ] **Update go.mod dependencies** when PR #419 merges
  ```bash
  go get github.com/metallb/frr-k8s@<version-with-evpn>
  ```

### Phase 2: Network Stretching (Later) - **BLOCKED on OVN-K**

**Dependencies (all WIP in OVN-K)**:
- ⏳ **VTEP Controller**: https://github.com/ovn-org/ovn-kubernetes/pull/6078
  - Implements VTEP lifecycle management in cluster-manager
  - Currently supports "unmanaged" VTEPs only
  - "managed" VTEPs coming in follow-up PR
  - **Critical**: Without this, VTEP CRs won't configure actual EVPN tunnels
- ⏳ **FRR-K8s EVPN API**: https://github.com/metallb/frr-k8s/pull/419
- ⏳ **OVN-K EVPN Integration**: https://github.com/ovn-org/ovn-kubernetes/pull/6127

**SkyNet Tasks (after OVN-K PRs merge)**:
- [ ] **Fix VTEP IP allocation** - Read from VTEP status instead of manual allocation
- [ ] Implement MultiClusterNetwork creation workflow
- [ ] Implement CUDN integration (inject VNI/RT into UserDefinedNetwork)
- [ ] Create RouteAdvertisement CRs for CUDN subnets
- [ ] Test end-to-end connectivity between stretched networks

---

## Testing Plan

### Phase 1: BGP Peering (Current - Ready to Test)

**Status**: ✓ Code changes complete, waiting for FRR-K8s merge

1. Deploy two clusters with updated code
2. Verify BGP sessions establish
3. Check FRRConfiguration has `addressFamilies: ["unicast", "evpn"]`
4. Verify EVPN address family is activated (even though no VNIs yet)

**Commands**:
```bash
# After deploying
kubectl --kubeconfig output/kubeconfig-cluster1.yaml \
  get frrconfiguration -n frr-k8s-system skynet-bgp-config -oyaml

# Verify EVPN address family in FRR
kubectl --kubeconfig output/kubeconfig-cluster1.yaml \
  exec -n frr-k8s-system <frr-pod> -c frr -- \
  vtysh -c 'show bgp l2vpn evpn summary'
```

### Phase 2: Network Stretching (Future)

1. Create MultiClusterNetwork on broker
2. Create MultiClusterNetworkConnect on local cluster
3. Verify L2VNI appears in FRRConfiguration
4. Verify EVPN routes are advertised
5. Test pod-to-pod connectivity across clusters

---

## OVN-K Dependencies for Phase 2

### VTEP Controller (PR #6078) - **CRITICAL**

**What it does**:
- Watches VTEP CRs and manages their lifecycle
- **In "managed" mode**: Allocates VTEP IP addresses per node from the VTEP CIDR
- Configures OVN to create actual EVPN tunnels based on VTEP spec
- Manages CUDN-to-VTEP associations
- Handles node endpoint discovery and tunnel setup
- Updates VTEP status with allocated node IPs

**Why we need it**:
- Without VTEP controller, VTEP CRs are just metadata
- No actual EVPN tunnels would be configured in OVN
- **CRITICAL for SkyNet**: We use "managed" mode - OVN-K allocates per-node IPs, not us
- CUDNs wouldn't be stretched across clusters

**Current status**:
- ⏳ WIP in PR #6078: Supports "unmanaged" VTEPs only
- ⏳ "managed" VTEPs coming in follow-up PR
- ⚠️ **SkyNet requires "managed" mode** - we create VTEPs with spec: {cidrs: ["100.x.0.0/16"], mode: "Managed"}

**Impact on SkyNet**:
- ✅ Phase 1 (BGP peering): No dependency - can test now
- ⏳ Phase 2 (network stretching): **BLOCKED** until VTEP controller merges
- ⚠️ **Code issue**: Our `vtep_manager.go:AllocateVtepIP()` manually allocates IPs
  - This is wrong for managed mode - OVN-K's VTEP controller does this
  - Need to read per-node IPs from VTEP status instead
  - Fix required for Phase 2

---

## SkyNet Code Issues for Phase 2

### VTEP IP Allocation - Needs Refactoring

**Current Implementation** (`pkg/agent/vtep/vtep_manager.go`):
```go
// AllocateVtepIP manually allocates IPs from VTEP CIDR
func (m *VtepManager) AllocateVtepIP(nodeName string) (string, error) {
    // Calculates 100.0.0.1, 100.0.0.2, etc.
    ip[len(ip)-1] = byte(m.vtepIPIndex)
    m.vtepIPIndex++
    return ip.String(), nil
}
```

**Problem**:
- In **managed mode**, OVN-K's VTEP controller allocates per-node VTEP IPs, not us
- Our manual allocation conflicts with the design
- We're using these IPs for endpoint reporting to broker

**Correct Implementation for Managed Mode**:
1. Create VTEP CR with `spec: {cidrs: ["100.x.0.0/16"], mode: "Managed"}`
2. VTEP controller allocates IPs and updates `status.nodeVtepIPs` map:
   ```yaml
   status:
     nodeVtepIPs:
       node1: 100.0.0.1
       node2: 100.0.0.2
   ```
3. SkyNet reads IPs from VTEP status for endpoint reporting

**Required Changes**:
- [ ] Replace `AllocateVtepIP()` with `GetNodeVtepIP(nodeName)` that reads from VTEP status
- [ ] Update `endpoint_reporter.go` to use new method
- [ ] Wait for VTEP controller to populate status before reporting endpoints

**Timeline**: Fix after OVN-K VTEP controller PR #6078 merges (defines status schema)

---

## Known Limitations & Workarounds

### BGP Listening Port (CRITICAL FIX REQUIRED)

**Issue**: FRR-K8s evpn-plan branch has `bgpd_options="-A 0.0.0.0 -p 0"` where `-p 0` means **"do not listen"**.

**Impact**: BGP daemon does NOT bind to port 179, preventing incoming connections. All sessions stuck in "Active" state.

**Fix**: Automatically applied in `test/setup-clusters-v2.sh`:
```bash
# Change -p 0 to -p 179 in frr-k8s-frr-startup ConfigMap
sed 's/-p 0/-p 179/g'
# Then restart FRR daemons
```

**Status**: ✅ Fixed in setup script. See `docs/FRR_K8S_BGP_PORT_FIX.md` for details.

**Upstream**: Should be reported to jcaamano/frr-k8s evpn-plan branch.

---

## References

- **FRR-K8s EVPN PR**: https://github.com/metallb/frr-k8s/pull/419
- **OVN-K EVPN PR**: https://github.com/ovn-org/ovn-kubernetes/pull/6127
- **FRR EVPN Docs**: https://docs.frrouting.org/en/latest/evpn.html
- **SkyNet Memory**: `/home/yboaron/.claude/projects/-home-yboaron-prj-skynet/memory/MEMORY.md`

---

## Summary

**Current Status**:
- ✅ BGP configurator code updated for EVPN API
- ✅ Builds successfully
- ✅ FRR-K8s deployment integrated via OVN-K PR #6127
- ✅ Ready to test Phase 1 immediately

**What Can Be Tested Now (Phase 1)**:
```bash
make deploy
make verify-bgp
```
**Expected Results**:
- ✅ BGP sessions establish between clusters
- ✅ Neighbors configured with `addressFamilies: ["unicast", "evpn"]`
- ✅ L2VPN EVPN address family activated (visible in FRR vtysh)
- ⚠️ No EVPN routes yet (no VNIs configured - that's Phase 2)

**What Is Blocked (Phase 2 - Network Stretching)**:
- ⏳ VTEP Controller (OVN-K PR #6078) - **CRITICAL**
- ⏳ FRR-K8s EVPN API (PR #419)
- ⏳ OVN-K EVPN Integration (PR #6127)

**Next Steps**:
1. ✅ **Test Phase 1 now** - BGP peering infrastructure
2. ⏳ Monitor OVN-K VTEP controller PR #6078
3. ⏳ Wait for all upstream PRs to merge
4. 🔜 Implement Phase 2 when OVN-K support is ready

**Blockers for Phase 2**: OVN-K VTEP controller (PR #6078)
