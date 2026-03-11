# SkyNet Local Test Environment

This directory contains scripts and configurations for testing SkyNet multi-cluster networking locally using KIND (Kubernetes IN Docker).

## Overview

The test environment consists of:
- **2 KIND clusters** (`cluster1` and `cluster2`) with OVN-Kubernetes CNI
- **FRR-K8s** for BGP/EVPN support
- **Broker** (using `cluster1` as the broker cluster)
- **SkyNet agents** deployed on both clusters

## Prerequisites

1. **Docker** - Running and accessible

2. **KIND** - Kubernetes IN Docker
   ```bash
   curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
   chmod +x ./kind
   sudo mv ./kind /usr/local/bin/kind
   ```

3. **kubectl** - Kubernetes CLI
   ```bash
   curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
   chmod +x kubectl
   sudo mv kubectl /usr/local/bin/
   ```

4. **git** - For cloning FRR-K8s
   ```bash
   sudo apt-get install git  # or yum install git
   ```

5. **OVN-Kubernetes Repository** - Required for cluster creation
   ```bash
   git clone https://github.com/ovn-org/ovn-kubernetes /path/to/ovn-kubernetes
   export OVNK_REPO_PATH=/path/to/ovn-kubernetes
   ```

   Or use existing repo:
   ```bash
   export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
   ```

## Quick Start

### 0. Check Prerequisites (Optional)

```bash
cd test
./check-prereqs.sh
```

This will verify all required tools are installed and OVNK_REPO_PATH is set correctly.

### 1. Setup the Clusters

First, set the OVN-K repository path:
```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
# Or wherever you have ovn-kubernetes cloned
```

Then run the setup:
```bash
cd test
chmod +x *.sh
./setup-clusters.sh
```

This script will:
- Create 2 KIND clusters using OVN-K's kind.sh (with interconnect enabled)
- Each cluster has 3 nodes (1 control-plane, 2 workers)
- Install OVN-Kubernetes CNI on both clusters
- Install FRR-K8s (v0.0.21 with OVN-K patches) for BGP support
- Setup broker namespace on cluster1
- Deploy SkyNet CRDs to the broker
- Create RBAC for agents on both clusters
- Generate broker access tokens
- Save kubeconfig files

**Time**: ~10-15 minutes depending on your system

### 2. Build the Agent Container

```bash
cd ..  # Back to project root
docker build -f test/Dockerfile -t skynet-agent:latest .
```

### 3. Load the Image to KIND Clusters

```bash
kind load docker-image skynet-agent:latest --name cluster1
kind load docker-image skynet-agent:latest --name cluster2
```

### 4. Deploy the Agents

```bash
cd test
./deploy-agents.sh
```

This deploys the SkyNet agent to both clusters.

### 5. Verify the Setup

```bash
./verify-setup.sh
```

This checks:
- Agent pod status
- Cluster registration on broker
- CRD installation
- Agent logs

### 6. Create a MultiClusterNetwork

```bash
./create-mcn.sh
```

This creates a test MultiClusterNetwork with VNI 5000 on the broker.

### 7. Connect Namespaces to the MCN

```bash
./create-mcnc.sh test-mcn default
```

This creates MultiClusterNetworkConnect resources in the `default` namespace of both clusters.

## Cluster Configuration

### Cluster 1
- **Name**: cluster1
- **Role**: Member cluster + Broker
- **Nodes**: 3 (1 control-plane, 2 workers)
- **Pod CIDR**: 10.244.0.0/16
- **Service CIDR**: 10.96.0.0/16
- **Context**: kind-cluster1

### Cluster 2
- **Name**: cluster2
- **Role**: Member cluster
- **Nodes**: 3 (1 control-plane, 2 workers)
- **Pod CIDR**: 10.245.0.0/16
- **Service CIDR**: 10.97.0.0/16
- **Context**: kind-cluster2

## Useful Commands

### Check Agent Status

```bash
# Cluster 1
kubectl --context kind-cluster1 -n skynet-system get pods
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent -f

# Cluster 2
kubectl --context kind-cluster2 -n skynet-system get pods
kubectl --context kind-cluster2 -n skynet-system logs -l app=skynet-agent -f
```

### Check Broker Resources

```bash
# Registered clusters
kubectl --context kind-cluster1 -n skynet-broker get clusters -o wide

# MultiClusterNetworks
kubectl --context kind-cluster1 -n skynet-broker get multiclusternetworks

# Cluster details (with ASN and VTEP CIDR)
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml
kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml
```

### Check Local Resources

```bash
# VTEPs (created for remote clusters)
kubectl --context kind-cluster1 get vteps
kubectl --context kind-cluster2 get vteps

# FRRConfigurations (BGP config)
kubectl --context kind-cluster1 get frrconfigurations
kubectl --context kind-cluster2 get frrconfigurations

# MultiClusterNetworkConnect
kubectl --context kind-cluster1 get mcnc --all-namespaces
kubectl --context kind-cluster2 get mcnc --all-namespaces

# UserDefinedNetworks (created by MCNC controller)
kubectl --context kind-cluster1 get udn --all-namespaces
kubectl --context kind-cluster2 get udn --all-namespaces
```

### Switch Between Clusters

```bash
kubectl config use-context kind-cluster1
kubectl config use-context kind-cluster2
```

## Testing Workflow

1. **Verify agents are running**:
   ```bash
   ./verify-setup.sh
   ```

2. **Check cluster registration**:
   ```bash
   kubectl --context kind-cluster1 -n skynet-broker get clusters
   ```
   You should see both `cluster1` and `cluster2` with their allocated ASN and VTEP CIDR.

3. **Create a MultiClusterNetwork**:
   ```bash
   ./create-mcn.sh
   ```

4. **Connect namespaces**:
   ```bash
   ./create-mcnc.sh test-mcn test-namespace
   ```

5. **Check VTEP creation**:
   ```bash
   kubectl --context kind-cluster1 get vteps
   kubectl --context kind-cluster2 get vteps
   ```
   Each cluster should have a VTEP for the remote cluster.

6. **Check BGP configuration**:
   ```bash
   kubectl --context kind-cluster1 get frrconfigurations -o yaml
   ```
   Should show BGP neighbors from the remote cluster.

7. **Check UDN creation**:
   ```bash
   kubectl --context kind-cluster1 -n test-namespace get udn
   kubectl --context kind-cluster2 -n test-namespace get udn
   ```

## Expected Results

After successful deployment:

1. **Cluster Registration**:
   - Both clusters register on the broker
   - Each cluster gets unique ASN (64512, 64513, etc.)
   - Each cluster gets unique VTEP CIDR (100.0.0.0/16, 100.1.0.0/16, etc.)

2. **Endpoint Reporting**:
   - Cluster CR status shows node endpoints with BGP peer IPs and VTEP IPs

3. **VTEP Resources**:
   - Each cluster creates a VTEP CR for the remote cluster
   - VTEP contains remote node endpoints

4. **BGP Configuration**:
   - FRRConfiguration created with eBGP neighbors
   - VRFs configured with route targets

5. **Namespace Connectivity**:
   - MCNC controller creates UserDefinedNetwork in connected namespaces
   - UDN configured based on MCN topology (Layer2/Layer3)

## Troubleshooting

### Agent Not Starting

Check logs:
```bash
kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent
```

Common issues:
- Broker token invalid
- Network connectivity to broker
- RBAC permissions

### Cluster Not Registering

Check:
1. Agent logs for errors
2. Broker namespace exists
3. CRDs installed on broker
4. Service account tokens valid

### VTEPs Not Created

Check:
1. Remote cluster registered on broker
2. Agent reconciliation loop running
3. Agent has permissions to create VTEPs

### Debug Mode

Deploy agent with higher verbosity:
```bash
# Edit deploy-agents.sh and change --v=4 to --v=10
./deploy-agents.sh
```

## Cleanup

To remove all test resources:

```bash
./cleanup.sh
```

This will:
- Delete both KIND clusters
- Remove generated token files
- Remove kubeconfig files
- Remove docker image

## File Structure

```
test/
├── README.md                    # This file
├── kind-cluster1.yaml          # KIND config for cluster1
├── kind-cluster2.yaml          # KIND config for cluster2
├── Dockerfile                  # Agent container image
├── setup-clusters.sh           # Setup script
├── deploy-agents.sh            # Deploy agents script
├── create-mcn.sh               # Create MultiClusterNetwork
├── create-mcnc.sh              # Create MultiClusterNetworkConnect
├── verify-setup.sh             # Verification script
└── cleanup.sh                  # Cleanup script
```

## Notes

- The test environment uses cluster1 as both a member cluster and the broker
- In production, the broker would typically be a separate cluster
- OVN-Kubernetes installation is manual and optional for basic testing
- Full connectivity testing requires OVN-K and FRR to be installed

## Next Steps

After verifying the basic setup:

1. Install OVN-Kubernetes on both clusters
2. Install FRR-K8s for BGP support
3. Deploy test pods in connected namespaces
4. Test cross-cluster pod connectivity
5. Verify EVPN route exchange
6. Test with multiple MultiClusterNetworks
7. Test Route Reflector topology
