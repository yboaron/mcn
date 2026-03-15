# SkyNet BGP Testing - Quick Start

**Goal**: Get BGP full mesh peering working between 2 OVN-K clusters.

## Prerequisites

```bash
# 1. Set OVN-K repo path
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes

# 2. Verify prerequisites
cd /home/yboaron/prj/skynet/test
./check-prereqs.sh
```

## Setup (One-Time)

### Step 1: Create Clusters with OVN-K + FRR-K8s

```bash
cd /home/yboaron/prj/skynet/test

# This takes ~10-15 minutes
./setup-clusters.sh
```

This creates:
- 2 KIND clusters (cluster1, cluster2) with 3 nodes each
- OVN-Kubernetes installed on both
- FRR-K8s installed on both
- Broker namespace on cluster1
- RBAC configured
- Broker access tokens generated

### Step 2: Build and Deploy Agent

```bash
cd /home/yboaron/prj/skynet

# Build agent binary
go build -o bin/skynet-agent ./cmd/agent

# Build and load Docker image
docker build -f test/Dockerfile -t skynet-agent:latest .
kind load docker-image skynet-agent:latest --name cluster1
kind load docker-image skynet-agent:latest --name cluster2

# Deploy agents
cd test
./deploy-agents.sh
```

### Step 3: Verify BGP Setup

```bash
cd /home/yboaron/prj/skynet/test

# Run comprehensive verification
./verify-bgp.sh
```

## What to Expect

### ✅ Successful Deployment

After running `verify-bgp.sh`, you should see:

```
=== Phase 1: Agent Status ===
[✓] Cluster1 agent is Running
[✓] Cluster2 agent is Running

=== Phase 2: Cluster Registration on Broker ===
[✓] Cluster1 registered on broker
[✓] Cluster2 registered on broker
[✓] ASN allocation: cluster1=64512, cluster2=64513 (unique ✓)
[✓] VTEP CIDR allocation: cluster1=100.0.0.0/16, cluster2=100.1.0.0/16 (unique ✓)
[✓] Cluster1 has 3 node endpoint(s) reported
[✓] Cluster2 has 3 node endpoint(s) reported

=== Phase 3: VTEP Resources ===
[✓] Cluster1 has VTEP for cluster2
[✓] Cluster2 has VTEP for cluster1
[✓] Cluster1's VTEP for cluster2 has 3 endpoint(s)

=== Phase 4: FRRConfiguration (BGP Config) ===
[✓] Cluster1 has FRRConfiguration
[✓] Cluster2 has FRRConfiguration
[✓] Cluster1 FRRConfiguration has 3 BGP neighbor(s)
[✓] Cluster2 FRRConfiguration has 3 BGP neighbor(s)
[✓] Cluster1 FRRConfiguration has L2VPN EVPN address family

=== Verification Summary ===
All checks passed! ✓
```

### 🔍 Checking BGP Sessions

Once verification passes, check actual BGP sessions in FRR:

```bash
# Get FRR pod on cluster1
FRR_POD=$(kubectl --context kind-cluster1 -n frr-k8s-system get pod -l app.kubernetes.io/name=frr-k8s -o name | head -1)

# Check BGP summary
kubectl --context kind-cluster1 -n frr-k8s-system exec -it $FRR_POD -- vtysh -c 'show bgp summary'

# Check EVPN specific summary
kubectl --context kind-cluster1 -n frr-k8s-system exec -it $FRR_POD -- vtysh -c 'show bgp l2vpn evpn summary'

# Check BGP neighbors detail
kubectl --context kind-cluster1 -n frr-k8s-system exec -it $FRR_POD -- vtysh -c 'show bgp neighbors'

# Check EVPN routes (will be empty until we add CUDN stretching)
kubectl --context kind-cluster1 -n frr-k8s-system exec -it $FRR_POD -- vtysh -c 'show bgp l2vpn evpn'
```

**Expected BGP Summary Output**:
```
Neighbor        V         AS   MsgRcvd   MsgSent   TblVer  InQ OutQ  Up/Down State/PfxRcd
192.168.2.10    4      64513        10        12        0    0    0 00:05:23            0
192.168.2.11    4      64513        10        12        0    0    0 00:05:23            0
192.168.2.12    4      64513        10        12        0    0    0 00:05:23            0

Total number of neighbors 3
```

## Troubleshooting

### Agent Not Starting

```bash
# Check pod status
kubectl --context kind-cluster1 -n skynet-system get pods -l app=skynet-agent

# Check logs
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent --tail=100

# Common issues:
# - Image not loaded: kind load docker-image skynet-agent:latest --name cluster1
# - Broker token missing: Run setup-clusters.sh again
```

### Cluster Not Registering

```bash
# Check if broker namespace exists
kubectl --context kind-cluster1 get namespace skynet-broker

# Check if CRDs are installed on broker
kubectl --context kind-cluster1 get crds | grep skynet

# Check agent logs for registration errors
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i register
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i error
```

### VTEPs Not Created

```bash
# Check if remote cluster is registered
kubectl --context kind-cluster1 -n skynet-broker get clusters

# Check if remote cluster has endpoints
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o jsonpath='{.status.endpoints}'

# Check agent reconciliation logs
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i vtep
```

### FRRConfiguration Not Created

```bash
# Check if VTEPs exist (prerequisite)
kubectl --context kind-cluster1 get vteps

# Check FRR-K8s is installed
kubectl --context kind-cluster1 get crds | grep frr

# Check agent logs
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i frr
```

### BGP Sessions Not Establishing

```bash
# Check FRRConfiguration was applied
kubectl --context kind-cluster1 get frrconfiguration skynet-bgp-config -o yaml

# Check FRR pods are running
kubectl --context kind-cluster1 -n frr-k8s-system get pods

# Check FRR logs
kubectl --context kind-cluster1 -n frr-k8s-system logs -l app.kubernetes.io/name=frr-k8s

# Check if nodes can reach each other (KIND networking)
# Get node IPs from VTEP or Cluster CR and test connectivity
```

## Useful Commands

### View All Resources

```bash
# Broker resources
kubectl --context kind-cluster1 -n skynet-broker get clusters
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml

# Local cluster resources
kubectl --context kind-cluster1 get vteps
kubectl --context kind-cluster1 get vtep cluster2 -o yaml
kubectl --context kind-cluster1 get frrconfigurations
kubectl --context kind-cluster1 get frrconfiguration skynet-bgp-config -o yaml
```

### Agent Logs

```bash
# Watch logs in real-time
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent -f

# Filter for specific components
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep "VTEP"
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep "BGP"
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep "Cluster"

# Show only errors
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent | grep -i error
```

### Restart Components

```bash
# Restart agent on cluster1
kubectl --context kind-cluster1 -n skynet-system rollout restart deployment skynet-agent

# Restart agent on cluster2
kubectl --context kind-cluster2 -n skynet-system rollout restart deployment skynet-agent

# Restart FRR (if needed)
kubectl --context kind-cluster1 -n frr-k8s-system delete pod -l app.kubernetes.io/name=frr-k8s
```

### Debug Mode

```bash
# Increase agent verbosity
kubectl --context kind-cluster1 -n skynet-system edit deployment skynet-agent

# Change:
#   args:
#   - --cluster-id=cluster1
#   - --v=4    # Change to --v=10 for debug

# Or use patch:
kubectl --context kind-cluster1 -n skynet-system patch deployment skynet-agent --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--cluster-id=cluster1", "--broker-server=https://cluster1-control-plane:6443", "--broker-token=$(cat /path/to/token)", "--broker-namespace=skynet-broker", "--v=10"]}]'
```

## Cleanup

```bash
cd /home/yboaron/prj/skynet/test
./cleanup.sh
```

This removes:
- Both KIND clusters
- Docker images
- Generated token files
- Kubeconfig files

## Next Steps

After BGP peering is working (Phase 1 complete):

1. **Phase 2**: Implement CUDN Integrator for network stretching
2. **Phase 3**: Test MultiClusterNetworkConnect
3. **Phase 4**: Verify EVPN route exchange for CUDNs
4. **Phase 5**: Test cross-cluster pod connectivity

## Current Status

✅ **Implemented**:
- Cluster registration with ASN and VTEP CIDR allocation
- VTEP resource creation for remote clusters
- FRRConfiguration generation for BGP full mesh
- Agent reconciliation loops
- Broker synchronization

⏳ **Not Yet Implemented** (focus of Phase 2+):
- CUDN integration
- MultiClusterNetworkConnect processing
- RouteAdvertisement creation
- Cross-cluster pod connectivity

## Reference

- **BGP Testing Plan**: `test/BGP_TESTING_PLAN.md`
- **Full README**: `test/README.md`
- **Test Environment**: `TEST_ENVIRONMENT.md`
- **OKEP-5088**: OVN-K EVPN support documentation
