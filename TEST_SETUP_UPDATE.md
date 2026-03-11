# Test Environment Updated to Use OVN-Kubernetes KIND Scripts

## Summary

The test environment setup has been updated to use the OVN-Kubernetes repository's `contrib/kind.sh` script for cluster creation, providing clusters with OVN-K and FRR-K8s pre-installed.

## Changes Made

### 1. Updated Cluster Creation Approach

**Before:**
- Simple KIND clusters with basic config
- Manual OVN-K installation required
- No FRR-K8s

**After:**
- Uses OVN-K `contrib/kind.sh` for cluster creation
- OVN-Kubernetes automatically installed with interconnect enabled
- FRR-K8s (v0.0.21) automatically installed with OVN-K patches
- Clusters ready for multi-cluster networking out of the box

### 2. New Prerequisites

- **OVN-Kubernetes Repository**: Required for cluster creation
  ```bash
  export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
  ```

### 3. Files Updated

#### `test/setup-clusters.sh`
- Now uses `$OVNK_REPO_PATH/contrib/kind.sh` to create clusters
- Installs FRR-K8s with OVN-K patches from https://github.com/jcaamano/frr-k8s
- Based on: https://github.com/yboaron/ovn-bgp-mcn-udn-poc/blob/main/scripts/0-create-clusters.sh
- FRR-K8s installation based on: https://github.com/yboaron/ovn-bgp-mcn-udn-poc/blob/main/scripts/0a-install-frr-k8s.sh

#### New Files Created
- `test/check-prereqs.sh` - Validates prerequisites and OVNK_REPO_PATH
  ```bash
  ./check-prereqs.sh
  ```

#### Files Removed
- `test/kind-cluster1.yaml` - No longer needed (OVN-K kind.sh creates clusters dynamically)
- `test/kind-cluster2.yaml` - No longer needed

#### Documentation Updated
- `test/README.md` - Added OVN-K repo requirement
- `test/QUICKSTART.md` - Updated prerequisites
- `TEST_ENVIRONMENT.md` - Updated with new setup flow

### 4. What Gets Installed Now

When you run `./setup-clusters.sh`, you get:

1. **Two KIND Clusters** (cluster1, cluster2)
   - 3 nodes each (1 control-plane, 2 workers)
   - Created using OVN-K contrib/kind.sh with `-ic` (interconnect)

2. **OVN-Kubernetes CNI**
   - Installed automatically by kind.sh
   - Interconnect enabled for multi-cluster
   - All CRDs installed: VTEPs, UserDefinedNetworks, etc.

3. **FRR-K8s v0.0.21**
   - Installed with OVN-K specific patches
   - Provides BGP/EVPN functionality
   - FRRConfiguration and FRRNodeState CRDs

4. **SkyNet Infrastructure**
   - Broker namespace on cluster1
   - SkyNet CRDs on broker
   - RBAC for agents
   - Broker access tokens

## Usage

### Quick Start

```bash
# Set OVN-K repo path
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes

# Check prerequisites (optional)
cd test
./check-prereqs.sh

# Run full setup
./run-all.sh
```

### Step by Step

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes

# 1. Create clusters with OVN-K + FRR-K8s
cd test
./setup-clusters.sh        # ~10-15 minutes

# 2. Build and load agent
./build-and-load.sh         # ~2-3 minutes

# 3. Deploy agents
./deploy-agents.sh          # ~1 minute

# 4. Verify
./verify-setup.sh

# 5. Create test resources
./create-mcn.sh
./create-mcnc.sh test-mcn default
```

## Key Improvements

1. **Production-like Environment**: Using the same OVN-K kind.sh that OVN-K developers use
2. **Complete Stack**: OVN-K + FRR-K8s + SkyNet all configured
3. **Proven Approach**: Based on working POC at https://github.com/yboaron/ovn-bgp-mcn-udn-poc
4. **Faster Testing**: No manual OVN-K or FRR-K8s installation needed
5. **Consistent**: Same OVN-K version/config across test runs

## Environment Variables

### Required
- `OVNK_REPO_PATH` - Path to OVN-Kubernetes repository

### Optional
- `CLUSTER1_NAME` - Name for first cluster (default: cluster1)
- `CLUSTER2_NAME` - Name for second cluster (default: cluster2)
- `NUM_WORKERS` - Worker nodes per cluster (default: 2)
- `FRR_K8S_VERSION` - FRR-K8s version (default: v0.0.21)

## Examples

### Using Different Cluster Names

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
export CLUSTER1_NAME=west
export CLUSTER2_NAME=east
./setup-clusters.sh
```

### More Workers

```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
export NUM_WORKERS=3
./setup-clusters.sh
```

## Verification After Setup

```bash
# Check OVN-K pods
kubectl --context kind-cluster1 get pods -n ovn-kubernetes

# Check FRR-K8s pods
kubectl --context kind-cluster1 get pods -n frr-k8s-system

# Check OVN-K CRDs
kubectl --context kind-cluster1 get crds | grep ovn

# Check FRR-K8s CRDs
kubectl --context kind-cluster1 get crds | grep frr
```

## Troubleshooting

### OVNK_REPO_PATH not set

```bash
Error: OVN-Kubernetes repo not found at:
```

**Solution:**
```bash
export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes
```

### Clusters taking long to create

This is normal. OVN-K installation takes 5-10 minutes per cluster.
FRR-K8s adds another 2-3 minutes.

### FRR-K8s patches fail to apply

This is usually OK - the patches may already be included in the version being used.
Check the logs; if FRR-K8s pods start successfully, it's fine.

## References

- **POC Repository**: https://github.com/yboaron/ovn-bgp-mcn-udn-poc
- **OVN-Kubernetes**: https://github.com/ovn-org/ovn-kubernetes
- **FRR-K8s**: https://github.com/metallb/frr-k8s
- **FRR-K8s with OVN-K patches**: https://github.com/jcaamano/frr-k8s/tree/ovnk-bgp-v0.0.21

## Next Steps

After setup completes:

1. Verify agents are running
2. Check cluster registration on broker
3. Verify VTEPs created for remote clusters
4. Check FRRConfigurations generated
5. Test cross-cluster connectivity (requires test pods)

See `test/README.md` for detailed testing procedures.
