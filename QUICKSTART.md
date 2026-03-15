# SkyNet Quick Start Guide

This guide shows how to quickly set up and test SkyNet multi-cluster networking with a single command.

## Prerequisites

- Docker installed and running
- `kind` (Kubernetes in Docker) installed
- `kubectl` installed
- Go 1.21+ (for building from source)

## One-Command Deployment

Create two OVN-Kubernetes clusters with FRR-K8s and deploy SkyNet agents:

```bash
make deploy
```

This command will:
1. Create 2 KIND clusters with OVN-Kubernetes CNI (non-overlapping CIDRs)
2. Install FRR-K8s on both clusters for BGP support
3. Build the SkyNet agent image
4. Deploy SkyNet agents to both clusters
5. Set up broker on cluster1

**Duration:** ~10-15 minutes (first run with image builds)

## Run End-to-End Tests

After deployment, verify everything is working:

```bash
make e2e
```

This runs the full e2e test suite including cluster registration and BGP verification.

## Step-by-Step (Alternative)

If you prefer to run steps individually:

```bash
# Step 1: Create clusters with OVN-K and FRR-K8s
make clusters

# Step 2: Build and load agent image
make build-agent

# Step 3: Deploy agents
make deploy-agents

# Step 4: Run e2e tests
make e2e
```

## Verify the Setup

### Check Agent Status
```bash
make agent-logs
```

### Check Cluster Registration
```bash
make broker-info
```

### Verify BGP Peering
```bash
make verify-bgp
```

### Check FRR Configurations
```bash
make frr-status
```

### Check VTEP Resources
```bash
make vtep-status
```

## Architecture

**Cluster Setup:**
- **cluster1**: 172.18.0.0/16, Pod CIDR: 10.244.0.0/16, Service CIDR: 10.96.0.0/12
- **cluster2**: 172.19.0.0/16, Pod CIDR: 10.245.0.0/16, Service CIDR: 10.97.0.0/12

**Components:**
- **Broker**: Runs on cluster1 in `skynet-broker` namespace
- **Agent**: Runs on both clusters in `skynet-operator` namespace
- **FRR-K8s**: BGP daemon for inter-cluster routing
- **OVN-K**: CNI providing VTEP and EVPN support

## Current Status

### ✅ Working
- Cluster creation with OVN-Kubernetes
- FRR-K8s installation and daemon running
- Broker setup and CRD installation
- Agent deployment and startup
- Cluster registration on broker
- ASN allocation (cluster1: 64512, cluster2: 64513)
- VTEP CIDR allocation (cluster1: 100.0.0.0/16, cluster2: 100.1.0.0/16)
- FRRConfiguration resource creation
- Heartbeat mechanism

### ⚠️ Known Issues
- **Endpoint synchronization**: Concurrent update conflict between heartbeat and endpoint status updates
  - Impact: BGP neighbors not populated in FRRConfiguration
  - Status: Identified, fix pending (needs retry logic or combined updates)
- **FRR rbac-proxy**: Image pull errors for `gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1`
  - Impact: None (FRR daemon and core containers running)

## Clean Up

Remove all clusters and resources:

```bash
make clean
```

## Development Workflow

### Code Generation
After modifying CRD types:
```bash
make codegen
```

### Running Tests
```bash
make test
```

### Running Linters
```bash
make lint
```

## Troubleshooting

### Check agent logs from specific cluster
```bash
# Cluster1
kubectl --context kind-cluster1 -n skynet-operator logs -l app=skynet-agent --tail=50

# Cluster2
kubectl --context kind-cluster2 -n skynet-operator logs -l app=skynet-agent --tail=50
```

### Check broker resources
```bash
kubectl --context kind-cluster1 -n skynet-broker get clusters -o yaml
```

### Check FRR daemon status
```bash
kubectl --context kind-cluster1 -n frr-k8s-system get pods
```

### Manual cleanup if make clean fails
```bash
kind delete cluster --name cluster1
kind delete cluster --name cluster2
rm -f test/kubeconfig-*.yaml test/broker-*.txt
```

## Next Steps

1. **Fix endpoint synchronization** - Resolve concurrent update conflicts
2. **Enable BGP peering** - Populate neighbors in FRRConfiguration
3. **Test CUDN stretching** - MultiClusterNetworkConnect functionality
4. **EVPN configuration** - Add VRF and route target support

## Getting Help

```bash
make help
```

This shows all available targets with descriptions.
