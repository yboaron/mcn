# Skynet - OVN-Kubernetes Multi-Cluster Networking (MCN)

## Project Goal

Our goal is to release a dev preview version of the OVN-Kubernetes Multi-Cluster Networking (MCN) solution based on BGP/EVPN. This solution targets customers seeking a simplified, declarative approach to connecting multiple OCP clusters via EVPN with minimal manual BGP configuration. The solution supports extending both the default network and ClusterUserDefinedNetworks (CUDNs) across clusters, with connectivity options for public-to-public (e.g., multi-region cloud deployments), public-to-private hybrid scenarios (e.g., connecting on-premises clusters to cloud-based OCP installations), and private-to-private connectivity (e.g., multiple on-premises datacenter clusters).

## Status

**Dev Preview** - Best-effort basis, no commitment to backwards/forwards compatibility.

## Architecture

Built on top of:
- [OKEP-5088 - EVPN Support](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/okeps/okep-5088-evpn.md)
- [OKEP-5296 - BGP Integration](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/okeps/okep-5296-bgp.md)

## Features

- **Multi-cluster connectivity** for both default network and ClusterUserDefinedNetworks (CUDNs)
  - **Default Network**: BGP route advertisement for pod-to-pod connectivity (no overlay)
  - **CUDNs**: EVPN-based Layer 2/Layer 3 network stretching for tenant isolation
- Layer 2 (MAC-VRF) and Layer 3 (IP-VRF) network extension across clusters
- VRF-based network isolation across clusters
- Declarative CRD-based configuration
- Integration with existing datacenter EVPN/BGP fabrics
- Support for VM migration scenarios via Layer 2 extension
- Public-to-public, public-to-private, and private-to-private cluster connectivity
- Minimal manual BGP configuration required

## Quick Start

### Deploy Test Environment

```bash
# Deploy 2 Kind clusters with OVN-K, FRR-K8s, and SkyNet agents
make deploy         # Takes ~7-8 minutes

# Verify BGP sessions established
make verify-bgp
```

### Test Default Network Connectivity

```bash
# Test default network pod connectivity across clusters (BGP route advertisement)
make test-default-network

# Cleanup default network test resources
make cleanup-default
```

**How it works:**
- Non-overlapping pod CIDRs configured automatically (cluster1: 10.244.0.0/16, cluster2: 10.245.0.0/16)
- RouteAdvertisement CR tells OVN-K to advertise pod network routes via BGP
- Direct IP routing - no EVPN/VXLAN overhead for default network
- Uses same mechanism as OVN-K no-overlay mode, extended across clusters

### Test CUDN Stretching

```bash
# Test Layer3 CUDN stretching across clusters (EVPN-based)
make test-cudn-l3   # Creates CUDN with EVPN, tests pod-to-pod connectivity

# Cleanup CUDN test resources
make cleanup-cudn
```

**How it works:**
- EVPN provides Layer 2/Layer 3 VPN for network isolation
- VNI allocation for tenant network separation
- VXLAN overlay for CUDN traffic

### Available Commands

- `make deploy` - Fresh deployment (clusters + BGP + agents)
- `make clusters` - Create Kind clusters only
- `make build-agent` - Build and load agent image
- `make deploy-agents` - Deploy SkyNet agents
- `make verify-bgp` - Verify BGP sessions
- `make test-default-network` - Test default network cross-cluster connectivity
- `make cleanup-default` - Clean default network test resources
- `make test-cudn-l3` - Test CUDN EVPN stretching
- `make cleanup-cudn` - Clean CUDN test resources
- `make clean` - Delete all Kind clusters

## Connectivity Modes

SkyNet supports two types of cross-cluster connectivity:

### 1. Default Network Connectivity (BGP Route Advertisement)

**Use case:** Pod-to-pod communication on the default Kubernetes network across clusters

**Technology:**
- BGP IPv4 unicast route advertisement via OVN-K RouteAdvertisement CRD
- Direct IP routing (no overlay encapsulation)
- Non-overlapping pod CIDRs required

**Advantages:**
- Lower latency (no VXLAN overhead)
- Simpler troubleshooting (standard IP routing)
- Uses OVN-K's native no-overlay mode mechanism

**How to use:**
```bash
make test-default-network
```

### 2. CUDN Stretching (EVPN-based)

**Use case:** Tenant network isolation and Layer 2/Layer 3 VPN across clusters

**Technology:**
- EVPN (Ethernet VPN) for network isolation
- VXLAN overlay for encapsulation
- VNI allocation per tenant network
- VRF-based multi-tenancy

**Advantages:**
- Network isolation (multi-tenancy)
- Overlapping IP addresses supported
- Layer 2 extension for VM migration

**How to use:**
```bash
make test-cudn-l3
```

## Architecture

The SkyNet agent runs in each cluster and handles:
- **MCNC Controller**: Reconciles MultiClusterNetworkConnect resources
- **CUDN Integrator**: Patches EVPN config into user-created CUDNs or creates new CUDNs
- **RouteAdvertisement Creator**: Triggers OVN-K BGP route advertisement
- **MCN Manager**: Manages MultiClusterNetwork lifecycle on broker
- **VNI Allocator**: Allocates unique VNI (5000-10000) per network
- **BGP Configurator**: Generates per-node FRRConfiguration for BGP mesh

## Workarounds (POC/Dev Preview)

This implementation includes temporary workarounds for upstream gaps:

1. **OVN-K fork required**: Uses `yboaron/ovn-kubernetes:skynet-evpn-base`
   - Removes CUDN `spec.network` immutability to allow EVPN patching
   - Fixes node subnet annotation during network Sync()

2. **FRR BGP listening address**: FRR-K8s configured to listen on `0.0.0.0` instead of `127.0.0.1`
   - Required for BGP peering between Kind container nodes
   - Production deployments use node IPs directly

2a. **BGP neighbor disableMP setting**: SkyNet sets `disableMP: true` in FRRConfiguration neighbors
   - Required for OVN-K RouteAdvertisement controller compatibility
   - Allows RouteAdvertisement CRD to inject pod network routes
   - When `disableMP: false`, FRR-K8s manages routes and blocks RouteAdvertisement controller

3. **VTEP IP assignment**: Setup script manually assigns VTEP IPs to node loopback
   - OVN-K VTEP controller `Managed` mode not yet supported upstream
   - Uses `Unmanaged` mode with pre-configured IPs

4. **VNI allocation race condition**: Kubernetes optimistic locking prevents conflicts for same MCN name
   - Theoretical race exists when creating different MCN names simultaneously across clusters
   - Both agents could allocate the same VNI for different networks, causing route leakage
   - Production solution: VNI range partitioning per cluster or broker-side locking

## License

_(To be added)_
