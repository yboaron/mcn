# SkyNet BGP Full Mesh Testing Plan

## Goal

Get BGP full mesh peering working between all nodes across 2 OVN-K clusters, focusing on cluster interconnection without CUDN stretching complexity.

## Test Environment

- **2 OVN-K KIND clusters** (already created via setup-clusters.sh)
- **FRR-K8s installed** on both clusters
- **Manual deployment** using kubectl apply (skip operator for now)

## Phase 1: Cluster BGP Peering Only

Focus on establishing BGP eBGP full mesh peering across all nodes.

### Architecture

```
Cluster1 (ASN 64512)              Cluster2 (ASN 64513)
├─ node1: 192.168.1.10           ├─ node1: 192.168.2.10
│  VTEP: 100.0.0.1               │  VTEP: 100.1.0.1
├─ node2: 192.168.1.11           ├─ node2: 192.168.2.11
│  VTEP: 100.0.0.2               │  VTEP: 100.1.0.2
└─ node3: 192.168.1.12           └─ node3: 192.168.2.12
   VTEP: 100.0.0.3                  VTEP: 100.1.0.3

Full Mesh eBGP Peering:
- Each node peers with ALL nodes in the other cluster
- eBGP multihop 255 (overlay network)
- Address family: l2vpn evpn
```

### What We're Testing

✅ **Cluster Registration**
- Agents register clusters on broker
- ASN allocation (64512, 64513)
- VTEP CIDR allocation (100.0.0.0/16, 100.1.0.0/16)

✅ **VTEP Creation**
- Each cluster creates VTEP CR for remote cluster
- VTEP endpoints populated from broker

✅ **BGP Configuration**
- FRRConfiguration generated with eBGP neighbors
- Full mesh topology: each node peers with all remote nodes
- L2VPN EVPN address family enabled

✅ **BGP Session Establishment**
- Verify BGP sessions using FRR vtysh
- Check EVPN route exchange

❌ **NOT Testing Yet**
- CUDN stretching
- MultiClusterNetworkConnect
- RouteAdvertisements
- Cross-cluster pod connectivity

## Prerequisites Check

```bash
# Check if clusters are running
kind get clusters
# Expected: cluster1, cluster2

# Check OVN-K is installed
kubectl --context kind-cluster1 get pods -n ovn-kubernetes
kubectl --context kind-cluster2 get pods -n ovn-kubernetes

# Check FRR-K8s is installed
kubectl --context kind-cluster1 get pods -n frr-k8s-system
kubectl --context kind-cluster2 get pods -n frr-k8s-system

# Check FRR CRDs
kubectl --context kind-cluster1 get crds | grep frr
# Expected: frrconfigurations.frrk8s.metallb.io, frrnodestates.frrk8s.metallb.io
```

## Deployment Steps

### Step 1: Build and Load Agent

```bash
cd /home/yboaron/prj/skynet

# Build agent binary
make build-agent

# Build Docker image
docker build -f test/Dockerfile -t skynet-agent:latest .

# Load into KIND clusters
kind load docker-image skynet-agent:latest --name cluster1
kind load docker-image skynet-agent:latest --name cluster2
```

### Step 2: Deploy Broker Components

```bash
# Use cluster1 as broker
kubectl config use-context kind-cluster1

# Create broker namespace
kubectl create namespace skynet-broker

# Apply SkyNet CRDs to broker
kubectl apply -f deploy/crds/

# Verify CRDs
kubectl get crds | grep skynet
```

### Step 3: Deploy Agents Manually

Use the existing deploy-agents.sh script or manual kubectl apply:

```bash
cd test
./deploy-agents.sh
```

This will:
- Apply CRDs to both clusters
- Create agent deployments with broker connection
- Configure agents with cluster IDs

### Step 4: Verify Agent Deployment

```bash
# Check agent pods
kubectl --context kind-cluster1 -n skynet-system get pods
kubectl --context kind-cluster2 -n skynet-system get pods

# Check agent logs
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent --tail=50
kubectl --context kind-cluster2 -n skynet-system logs -l app=skynet-agent --tail=50
```

Expected in logs:
```
Starting SkyNet Agent for cluster: cluster1
Registering cluster cluster1 with broker
Allocated VTEP CIDR 100.0.0.0/16 for cluster cluster1
Allocated ASN 64512 for cluster cluster1
Successfully created Cluster CR on broker
Runtime components initialized successfully
SkyNet Agent started successfully
```

## Verification Steps

### V1: Cluster Registration

```bash
# Check clusters registered on broker
kubectl --context kind-cluster1 -n skynet-broker get clusters

# Verify ASN and VTEP allocation
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml
```

Expected output:
```yaml
# cluster1
spec:
  clusterID: cluster1
  vtepCIDR: 100.0.0.0/16
  asn: 64512
status:
  phase: Ready
  endpoints:
  - node: cluster1-control-plane
    bgpPeerIP: <node-internal-ip>
    vtepIP: 100.0.0.1
  - node: cluster1-worker
    bgpPeerIP: <node-internal-ip>
    vtepIP: 100.0.0.2
  - node: cluster1-worker2
    bgpPeerIP: <node-internal-ip>
    vtepIP: 100.0.0.3
```

### V2: VTEP Resources Created

```bash
# Check VTEPs on cluster1 (should have VTEP for cluster2)
kubectl --context kind-cluster1 get vteps

# Check VTEP details
kubectl --context kind-cluster1 get vtep cluster2 -o yaml

# Same for cluster2
kubectl --context kind-cluster2 get vteps
kubectl --context kind-cluster2 get vtep cluster1 -o yaml
```

Expected: Each cluster has a VTEP CR for the remote cluster with all node endpoints.

### V3: FRRConfiguration Created

```bash
# Check FRRConfigurations on both clusters
kubectl --context kind-cluster1 get frrconfigurations
kubectl --context kind-cluster2 get frrconfigurations

# Check BGP config details
kubectl --context kind-cluster1 get frrconfiguration skynet-bgp-config -o yaml
```

Expected FRRConfiguration structure:
```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: skynet-bgp-config
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: <cluster2-node1-ip>
        asn: 64513
        ebgpMultiHop: 255
        addressFamilies:
        - afi: l2vpn
          safi: evpn
      - address: <cluster2-node2-ip>
        asn: 64513
        ebgpMultiHop: 255
        addressFamilies:
        - afi: l2vpn
          safi: evpn
      # ... more neighbors
```

### V4: BGP Sessions Established

Access FRR on a node and check BGP status:

```bash
# Get FRR pod name on cluster1
FRR_POD=$(kubectl --context kind-cluster1 -n frr-k8s-system get pods -l app.kubernetes.io/name=frr-k8s -o name | head -1)

# Access FRR vtysh
kubectl --context kind-cluster1 -n frr-k8s-system exec -it $FRR_POD -- vtysh

# Inside vtysh:
show bgp summary
show bgp l2vpn evpn summary
show bgp neighbors
```

Expected:
- BGP sessions to all cluster2 nodes in "Established" state
- Address family: L2VPN/EVPN

### V5: EVPN Routes

Check if EVPN routes are being exchanged:

```bash
# In vtysh:
show bgp l2vpn evpn
show evpn vni
```

Note: Without CUDN stretching, we won't see many routes yet, but the BGP infrastructure should be ready.

## Troubleshooting

### Issue: Agent Pod Not Starting

**Check:**
```bash
kubectl --context kind-cluster1 -n skynet-system describe pod -l app=skynet-agent
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent
```

**Common causes:**
- Image not loaded to KIND: `kind load docker-image skynet-agent:latest --name cluster1`
- RBAC issues: Check ServiceAccount permissions
- Broker token invalid: Regenerate with setup-clusters.sh

### Issue: Cluster Not Registering

**Check:**
- Broker namespace exists on cluster1
- CRDs installed on broker
- Agent has network access to broker API server
- Agent logs show connection errors

### Issue: VTEPs Not Created

**Check:**
- Both clusters registered on broker
- Agent reconciliation loop running (check logs)
- Remote cluster has endpoints populated in status

### Issue: FRRConfiguration Not Created

**Check:**
- Remote cluster endpoints available
- Agent has permissions to create FRRConfiguration
- FRR-K8s CRDs installed

### Issue: BGP Sessions Not Establishing

**Check:**
- FRRConfiguration applied correctly
- Node labels match nodeSelector in FRRConfiguration
- Network connectivity between cluster nodes (KIND networking)
- FRR daemon running: `kubectl get pods -n frr-k8s-system`

## Success Criteria

✅ **Phase 1 Complete when:**

1. Both clusters registered on broker with unique ASN and VTEP CIDR
2. Each cluster has VTEP CR for remote cluster
3. FRRConfiguration created on both clusters
4. BGP sessions established (verify with vtysh)
5. L2VPN EVPN address family enabled
6. No errors in agent logs

## Next Phase: CUDN Stretching

After Phase 1 is working:

1. Implement CUDN integrator
2. Test MultiClusterNetworkConnect
3. Create RouteAdvertisements
4. Verify EVPN route exchange for CUDNs
5. Test cross-cluster pod connectivity

## Useful Commands Reference

```bash
# Agent logs with filtering
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i error
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i "vtep"
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i "bgp"

# Watch agent status
kubectl --context kind-cluster1 -n skynet-system get pods -l app=skynet-agent -w

# Get all SkyNet resources
kubectl --context kind-cluster1 get clusters,multiclusternetworks -n skynet-broker
kubectl --context kind-cluster1 get vteps,frrconfigurations

# Restart agents
kubectl --context kind-cluster1 -n skynet-system rollout restart deployment skynet-agent
kubectl --context kind-cluster2 -n skynet-system rollout restart deployment skynet-agent
```

## Debug Mode

For more verbose logging:

```bash
# Edit deployment to increase verbosity
kubectl --context kind-cluster1 -n skynet-system edit deployment skynet-agent

# Change:  --v=4  to  --v=10

# Or use patch:
kubectl --context kind-cluster1 -n skynet-system patch deployment skynet-agent \
  --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args/4", "value": "--v=10"}]'
```
