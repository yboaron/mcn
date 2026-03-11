# SkyNet Test Environment - Quick Start

This is a step-by-step guide to test SkyNet locally.

## Prerequisites

Ensure you have:
- Docker installed and running
- KIND installed
- kubectl installed
- git installed
- Go 1.25+ installed (for building the agent)
- OVN-Kubernetes repository cloned

**Setup OVN-K Repository:**
```bash
git clone https://github.com/ovn-org/ovn-kubernetes /path/to/ovn-kubernetes
export OVNK_REPO_PATH=/path/to/ovn-kubernetes
```

Or use existing repo:
```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
```

## Option 1: Automated Setup (Recommended)

Run everything with a single command:

```bash
cd test
./run-all.sh
```

This will:
1. Create 2 KIND clusters
2. Setup broker
3. Build and load the agent image
4. Deploy agents
5. Create test MultiClusterNetwork
6. Create MultiClusterNetworkConnect
7. Verify the setup

## Option 2: Step-by-Step Setup

### Step 1: Setup Clusters

```bash
cd test
./setup-clusters.sh
```

This creates:
- `cluster1` (3 nodes: 1 control-plane, 2 workers) - also serves as broker
- `cluster2` (3 nodes: 1 control-plane, 2 workers)

### Step 2: Build and Load Agent Image

```bash
./build-and-load.sh
```

Or using make from the project root:
```bash
cd ..
make test-build-load
```

### Step 3: Deploy Agents

```bash
cd test
./deploy-agents.sh
```

Or:
```bash
make test-deploy
```

### Step 4: Verify Setup

```bash
./verify-setup.sh
```

Or:
```bash
make test-verify
```

### Step 5: Create MultiClusterNetwork

```bash
./create-mcn.sh
```

Or:
```bash
make test-mcn
```

### Step 6: Connect Namespaces

```bash
./create-mcnc.sh test-mcn default
```

Or:
```bash
make test-mcnc
```

## Using Make Commands

From the project root:

```bash
# Show help
make test-help

# Complete setup
make test-all

# Individual steps
make test-setup
make test-build-load
make test-deploy
make test-verify

# Monitoring
make agent-logs-c1      # Watch agent logs in cluster1
make broker-clusters    # Show registered clusters
make vteps-c1          # Show VTEPs in cluster1

# Cleanup
make test-clean
```

## What to Expect

After successful setup:

### 1. Cluster Registration

Both clusters should register on the broker:

```bash
kubectl --context kind-cluster1 -n skynet-broker get clusters
```

Expected output:
```
NAME       AGE
cluster1   2m
cluster2   2m
```

Check cluster details:
```bash
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml
```

You should see:
- `spec.asn`: Allocated ASN (e.g., 64512)
- `spec.vtepCIDR`: Allocated VTEP CIDR (e.g., 100.0.0.0/16)
- `status.endpoints`: List of nodes with BGP and VTEP IPs
- `status.phase`: "Ready"

### 2. VTEP Resources

Each cluster should create VTEPs for remote clusters:

```bash
kubectl --context kind-cluster1 get vteps
kubectl --context kind-cluster2 get vteps
```

Expected: One VTEP per remote cluster (e.g., `skynet-cluster2` in cluster1)

### 3. BGP Configuration

FRRConfigurations should be created:

```bash
kubectl --context kind-cluster1 get frrconfigurations
```

Expected: `skynet-bgp-config` with BGP neighbors from remote cluster

### 4. Namespace Connectivity

UserDefinedNetworks should be created in connected namespaces:

```bash
kubectl --context kind-cluster1 get udn -n default
kubectl --context kind-cluster2 get udn -n default
```

Expected: `skynet-test-mcn` UDN in both clusters

## Monitoring

### Watch Agent Logs

Cluster 1:
```bash
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent -f
```

Cluster 2:
```bash
kubectl --context kind-cluster2 -n skynet-system logs -l app=skynet-agent -f
```

### Check Agent Status

```bash
kubectl --context kind-cluster1 -n skynet-system get pods
kubectl --context kind-cluster2 -n skynet-system get pods
```

### Inspect Resources

```bash
# Clusters on broker
kubectl --context kind-cluster1 -n skynet-broker get clusters -o yaml

# MultiClusterNetworks
kubectl --context kind-cluster1 -n skynet-broker get mcn

# VTEPs
kubectl --context kind-cluster1 get vteps -o yaml

# FRRConfigurations
kubectl --context kind-cluster1 get frrconfigurations -o yaml

# MultiClusterNetworkConnect
kubectl --context kind-cluster1 get mcnc -A

# UserDefinedNetworks
kubectl --context kind-cluster1 get udn -A
```

## Troubleshooting

### Agent Not Starting

Check logs:
```bash
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent
```

Common issues:
- Image not loaded: Run `make test-build-load` again
- RBAC issues: Check serviceaccount permissions
- Broker connection: Verify broker-token and broker-server files exist

### Cluster Not Registering

1. Check agent logs for errors
2. Verify CRDs are installed:
   ```bash
   kubectl --context kind-cluster1 get crds | grep skynet
   ```
3. Check broker namespace exists:
   ```bash
   kubectl --context kind-cluster1 get ns skynet-broker
   ```

### VTEPs Not Created

1. Ensure remote cluster is registered
2. Check agent reconciliation in logs
3. Verify VTEP CRD is installed:
   ```bash
   kubectl --context kind-cluster1 get crds vteps.k8s.ovn.org
   ```

### Enable Debug Logging

Edit the agent deployment to increase verbosity:

```bash
kubectl --context kind-cluster1 -n skynet-system edit deployment skynet-agent
```

Change `--v=4` to `--v=10`

## Cleanup

Remove everything:

```bash
cd test
./cleanup.sh
```

Or:
```bash
make test-clean
```

This will:
- Delete both KIND clusters
- Remove generated files
- Remove docker image

## Next Steps

After verifying the basic setup:

1. **Install OVN-Kubernetes**: For full network functionality
2. **Install FRR-K8s**: For BGP/EVPN support
3. **Deploy test pods**: Verify cross-cluster connectivity
4. **Test multiple MCNs**: Create additional MultiClusterNetworks
5. **Test Route Reflector mode**: Configure route reflector topology

## File Reference

- `kind-cluster1.yaml` - KIND config for cluster1
- `kind-cluster2.yaml` - KIND config for cluster2
- `Dockerfile` - Agent container image
- `setup-clusters.sh` - Setup script
- `build-and-load.sh` - Build and load image
- `deploy-agents.sh` - Deploy agents
- `verify-setup.sh` - Verification script
- `create-mcn.sh` - Create MCN
- `create-mcnc.sh` - Create MCNC
- `run-all.sh` - Complete automated setup
- `cleanup.sh` - Cleanup script
