# SkyNet Test Environment Setup Complete

## What Was Created

A complete local test environment for SkyNet multi-cluster networking with 2 OVN-Kubernetes KIND clusters.

### Test Infrastructure Files

```
test/
├── README.md                    # Comprehensive test environment guide
├── QUICKSTART.md               # Quick start guide
├── Dockerfile                  # Agent container image
├── kind-cluster1.yaml          # KIND configuration for cluster1
├── kind-cluster2.yaml          # KIND configuration for cluster2
├── setup-clusters.sh           # Setup 2 clusters + broker
├── build-and-load.sh           # Build and load agent image
├── deploy-agents.sh            # Deploy agents to clusters
├── create-mcn.sh               # Create MultiClusterNetwork
├── create-mcnc.sh              # Create MultiClusterNetworkConnect
├── verify-setup.sh             # Comprehensive verification
├── run-all.sh                  # Automated complete setup
└── cleanup.sh                  # Tear down everything
```

### Makefile Targets Added

```makefile
make test-help          # Show test commands
make test-setup         # Setup clusters
make test-build-load    # Build and load image
make test-deploy        # Deploy agents
make test-verify        # Verify setup
make test-mcn           # Create MCN
make test-mcnc          # Create MCNC
make test-all           # Complete automated setup
make test-clean         # Cleanup

# Monitoring
make agent-logs-c1      # Watch cluster1 agent logs
make agent-logs-c2      # Watch cluster2 agent logs
make broker-clusters    # Show registered clusters
make broker-mcns        # Show MultiClusterNetworks
make vteps-c1          # Show VTEPs in cluster1
make vteps-c2          # Show VTEPs in cluster2
make frr-c1            # Show FRRConfigurations in cluster1
make frr-c2            # Show FRRConfigurations in cluster2
```

## Prerequisites

Before starting, ensure you have:
- Docker, KIND, kubectl, git installed
- OVN-Kubernetes repository cloned

```bash
# Clone or use existing OVN-K repo
git clone https://github.com/ovn-org/ovn-kubernetes /path/to/ovn-kubernetes
export OVNK_REPO_PATH=/path/to/ovn-kubernetes

# Or use existing:
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
```

## Quick Start

### Option 1: Automated (Recommended)

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
cd test
./run-all.sh
```

### Option 2: Using Make

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
make test-all
```

### Option 3: Step by Step

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes

# 1. Setup clusters (includes OVN-K + FRR-K8s installation)
make test-setup

# 2. Build and load agent
make test-build-load

# 3. Deploy agents
make test-deploy

# 4. Verify
make test-verify

# 5. Create MCN
make test-mcn

# 6. Connect namespaces
make test-mcnc
```

## Test Environment Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Broker Cluster                           │
│                        (cluster1 context)                        │
│                                                                   │
│  Namespace: skynet-broker                                        │
│  ├── CRDs: Cluster, MultiClusterNetwork, etc.                   │
│  ├── Cluster CR: cluster1 (ASN: 64512, VTEP: 100.0.0.0/16)     │
│  ├── Cluster CR: cluster2 (ASN: 64513, VTEP: 100.1.0.0/16)     │
│  └── MultiClusterNetwork: test-mcn (VNI: 5000)                  │
└─────────────────────────────────────────────────────────────────┘

┌──────────────────────────────┐  ┌──────────────────────────────┐
│       Cluster 1              │  │       Cluster 2              │
│    (kind-cluster1)           │  │    (kind-cluster2)           │
│                              │  │                              │
│  Namespace: skynet-system    │  │  Namespace: skynet-system    │
│  └── skynet-agent pod        │  │  └── skynet-agent pod        │
│                              │  │                              │
│  Resources:                  │  │  Resources:                  │
│  ├── VTEP: skynet-cluster2   │  │  ├── VTEP: skynet-cluster1   │
│  ├── FRRConfiguration        │  │  ├── FRRConfiguration        │
│  └── UDN: skynet-test-mcn    │  │  └── UDN: skynet-test-mcn    │
│                              │  │                              │
│  Allocated:                  │  │  Allocated:                  │
│  ├── ASN: 64512              │  │  ├── ASN: 64513              │
│  └── VTEP CIDR: 100.0.0.0/16 │  │  └── VTEP CIDR: 100.1.0.0/16 │
└──────────────────────────────┘  └──────────────────────────────┘
```

## What Gets Tested

### 1. Cluster Registration
- ✅ Agent connects to broker
- ✅ ASN allocation (optimistic locking)
- ✅ VTEP CIDR allocation (optimistic locking)
- ✅ Cluster CR creation on broker

### 2. Endpoint Reporting
- ✅ Node discovery
- ✅ BGP peer IP extraction
- ✅ VTEP IP allocation per node
- ✅ Cluster CR status update

### 3. VTEP Management
- ✅ VTEP CR creation for remote clusters
- ✅ Endpoint synchronization
- ✅ VTEP lifecycle management

### 4. BGP Configuration
- ✅ FRRConfiguration generation
- ✅ eBGP neighbor configuration
- ✅ VRF setup with route targets
- ✅ EVPN address family configuration

### 5. Namespace Connectivity
- ✅ MCNC resource reconciliation
- ✅ UserDefinedNetwork creation
- ✅ Topology configuration (Layer2/Layer3)

### 6. Resource Synchronization
- ✅ Broker syncer functionality
- ✅ Local to broker sync (Cluster CR)
- ✅ Broker to local sync (MCN)

## Verification Points

After running the setup, verify:

```bash
# 1. Agents are running
kubectl --context kind-cluster1 -n skynet-system get pods
kubectl --context kind-cluster2 -n skynet-system get pods

# 2. Clusters registered on broker
kubectl --context kind-cluster1 -n skynet-broker get clusters

# 3. Cluster details show ASN and VTEP CIDR
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml

# 4. VTEPs created for remote clusters
kubectl --context kind-cluster1 get vteps
kubectl --context kind-cluster2 get vteps

# 5. FRRConfigurations created
kubectl --context kind-cluster1 get frrconfigurations
kubectl --context kind-cluster2 get frrconfigurations

# 6. UserDefinedNetworks created in connected namespaces
kubectl --context kind-cluster1 get udn -n default
kubectl --context kind-cluster2 get udn -n default
```

## Expected Results

### Cluster Registration
```yaml
apiVersion: skynet.io/v1
kind: Cluster
metadata:
  name: cluster1
  namespace: skynet-broker
spec:
  clusterID: cluster1
  asn: 64512
  vtepCIDR: 100.0.0.0/16
status:
  phase: Ready
  lastHeartbeat: "2026-03-11T..."
  endpoints:
  - node: cluster1-control-plane
    bgpPeerIP: 172.18.0.3
    vtepIP: 100.0.0.1
    routeReflector: false
  - node: cluster1-worker
    bgpPeerIP: 172.18.0.4
    vtepIP: 100.0.0.2
    routeReflector: false
```

### VTEP Resource
```yaml
apiVersion: k8s.ovn.org/v1
kind: Vtep
metadata:
  name: skynet-cluster2
spec:
  name: skynet-cluster2
  endpoints:
  - ip: 172.18.0.6    # BGP peer IP
    vtep: 100.1.0.1   # VTEP IP
  - ip: 172.18.0.7
    vtep: 100.1.0.2
```

### FRRConfiguration
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
      - address: 172.18.0.6
        asn: 64513
        ebgpMultiHop: 255
        addressFamilies:
        - afi: l2vpn
          safi: evpn
    vrfs:
    - name: vrf-test-mcn
      vni: 5000
      importTargets: ["65000:5000"]
      exportTargets: ["65000:5000"]
```

## Troubleshooting

See `test/README.md` for detailed troubleshooting steps.

Quick checks:
```bash
# Agent logs
make agent-logs-c1

# Verify all components
make test-verify
```

## Cleanup

```bash
make test-clean
```

Or:
```bash
cd test
./cleanup.sh
```

## Next Steps

1. **Manual Testing**: Follow test/QUICKSTART.md
2. **OVN-K Integration**: Install OVN-Kubernetes for full networking
3. **FRR Integration**: Install FRR-K8s for BGP support
4. **Connectivity Test**: Deploy test pods and verify cross-cluster ping
5. **Multiple MCNs**: Test with multiple MultiClusterNetworks
6. **Route Reflector**: Test RR topology

## Documentation

- `test/README.md` - Comprehensive guide
- `test/QUICKSTART.md` - Quick start steps
- `IMPLEMENTATION_SUMMARY.md` - Implementation details

## Notes

- **Broker**: cluster1 serves as both member and broker
- **OVN-K**: Manual installation required for full testing
- **FRR**: Manual installation required for BGP/EVPN
- **CRDs**: VTEP and FRRConfiguration CRDs from external projects

All scripts are idempotent and can be run multiple times safely.
