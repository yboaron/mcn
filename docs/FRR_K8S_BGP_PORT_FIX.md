# FRR-K8s BGP Port 179 Listening Fix

## Issue

**Problem**: BGP sessions stuck in "Active" state, unable to establish peering between clusters.

**Root Cause**: FRR-K8s evpn-plan branch (jcaamano/frr-k8s) has a configuration bug in the `frr-k8s-frr-startup` ConfigMap:

```yaml
bgpd_options="   -A 0.0.0.0 -p 0 --limit-fds 100000"
                            ^^^^
                            BUG: -p 0 means "do not listen"
```

According to `bgpd --help`:
```
-p, --bgp_port  Set BGP listen port number (0 means do not listen).
```

When bgpd is started with `-p 0`, it:
- Does NOT bind to TCP port 179
- Can only initiate outgoing connections
- Cannot accept incoming BGP connections
- Results in both sides stuck in "Active" state (both trying to connect out, neither listening)

## Symptoms

```bash
# Check BGP status - all neighbors show "Active" state
kubectl exec <frr-pod> -c frr -- vtysh -c "show ip bgp summary"

# BGP daemon is running but not listening on port 179
kubectl exec <frr-pod> -c frr -- ss -tlnp | grep :179
# (no output - port 179 not listening)

# BGP logs show connection refused
kubectl logs <frr-pod> -c frr | grep -i "connection refused"
# BGP: [XB8WX-EQF0G] 172.18.0.7 [Event] Connect failed 111(Connection refused)
```

## Fix

Change `-p 0` to `-p 179` in the FRR-K8s startup ConfigMap:

```bash
# Get ConfigMap, replace -p 0 with -p 179, apply
kubectl -n frr-k8s-system get configmap frr-k8s-frr-startup -o yaml | \
    sed 's/-p 0/-p 179/g' | \
    kubectl apply -f -

# Restart FRR daemons to pick up the change
kubectl -n frr-k8s-system rollout restart daemonset frr-k8s-daemon
kubectl -n frr-k8s-system rollout status daemonset frr-k8s-daemon --timeout=180s
```

After the fix:
```bash
# BGP now listening on port 179
kubectl exec <frr-pod> -c frr -- ss -tlnp | grep :179
# LISTEN 0  128  0.0.0.0:179  0.0.0.0:*
# LISTEN 0  128     [::]:179     [::]:*

# BGP sessions established
kubectl exec <frr-pod> -c frr -- vtysh -c "show ip bgp summary"
# Neighbor   V   AS   MsgRcvd  MsgSent  State/PfxRcd
# 172.18.0.6 4  64513    38       39      0         (Established!)
```

## Automated Fix in SkyNet

The fix is automatically applied during cluster setup in `test/setup-clusters-v2.sh`:

```bash
apply_bgp_listening_workaround() {
    # Patch both -A and -p flags
    kubectl get configmap -n frr-k8s-system frr-k8s-frr-startup -o yaml | \
        sed -e 's/127\.0\.0\.1/0.0.0.0/g' -e 's/-p 0/-p 179/g' | \
        kubectl apply -f -

    kubectl rollout restart daemonset/frr-k8s-daemon -n frr-k8s-system
    kubectl rollout status daemonset/frr-k8s-daemon -n frr-k8s-system --timeout=180s
}
```

This ensures BGP is listening on port 179 on all interfaces (0.0.0.0) for proper multi-cluster peering.

## Upstream Status

**Branch**: jcaamano/frr-k8s evpn-plan
**Issue**: Configuration has `-p 0` preventing BGP listening
**Status**: Should be reported upstream

**Related PRs**:
- FRR-K8s EVPN API: https://github.com/metallb/frr-k8s/pull/419
- OVN-K EVPN Integration: https://github.com/ovn-org/ovn-kubernetes/pull/6127

## Testing

After applying the fix:

1. **Verify BGP is listening**:
   ```bash
   kubectl exec -n frr-k8s-system <frr-pod> -c frr -- ss -tlnp | grep :179
   ```

2. **Verify BGP sessions**:
   ```bash
   kubectl exec -n frr-k8s-system <frr-pod> -c frr -- vtysh -c "show ip bgp summary"
   ```

3. **Run full verification**:
   ```bash
   make verify-bgp
   ```

All BGP sessions should show "Established" state with messages being exchanged.

## Workaround History

1. **Initial workaround**: Patched 127.0.0.1 → 0.0.0.0 for `-A` flag (listen address)
2. **Discovered**: jcaamano/frr-k8s evpn-plan already has 0.0.0.0, so that patch was not needed
3. **Root cause found**: `-p 0` flag was preventing BGP from listening at all
4. **Final fix**: Patch both flags to be safe: `-A 0.0.0.0 -p 179`

## Impact

Without this fix:
- ❌ BGP sessions cannot establish
- ❌ No route exchange between clusters
- ❌ Multi-cluster networking non-functional

With this fix:
- ✅ BGP full mesh peering works
- ✅ EVPN address families activated
- ✅ Ready for Phase 2 (network stretching)
