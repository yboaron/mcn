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

- Multi-cluster connectivity for both default network and ClusterUserDefinedNetworks (CUDNs) via EVPN
- Layer 2 (MAC-VRF) and Layer 3 (IP-VRF) network extension across clusters
- VRF-based network isolation across clusters
- Declarative CRD-based configuration
- Integration with existing datacenter EVPN/BGP fabrics
- Support for VM migration scenarios via Layer 2 extension
- Public-to-public, public-to-private, and private-to-private cluster connectivity
- Minimal manual BGP configuration required

## Documentation

_(To be added)_

## Getting Started

_(To be added)_

## License

_(To be added)_
