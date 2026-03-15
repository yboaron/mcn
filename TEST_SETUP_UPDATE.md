# Test Environment Scripts - E2E Updates

## Summary

The test environment setup has been enhanced for reliability and CI/CD compatibility. The primary script **`setup-clusters-v2.sh`** now handles complete end-to-end setup with improved error handling, retry logic, and better user feedback.

### Major Improvements (Latest Update)

1. **VTEP CRD verification with retry logic** - Handles timing issues gracefully
2. **Consistent kubectl usage** - Standardized on `--context` throughout
3. **Enhanced FRR-K8s installation** - Better wait logic and error handling
4. **Improved user feedback** - Clear messages about created files and next steps
5. **Script organization** - Deprecated old script, clarified purpose of manual completion script

---

## Latest Updates - Incorporating Manual Setup Learnings

### Problem Statement

After manual completion of setup (via `complete-setup.sh`), we identified several issues in the automated `setup-clusters-v2.sh`:
- VTEP CRD verification failed due to timing (kubeconfig export is async)
- Inconsistent kubectl command usage (mix of `--context` and `--kubeconfig`)
- FRR-K8s wait commands could break the script
- Unclear output about what files were created

### Solutions Implemented

#### 1. VTEP CRD Verification - Retry Logic

**Before**: Single check that failed if kubeconfig wasn't ready
```bash
if kubectl --context "$cluster_ctx" get crd vteps.k8s.ovn.org &> /dev/null; then
    log_info "✓ VTEP CRD installed"
else
    log_error "VTEP CRD not found!"
    return 1
fi
```

**After**: Retry loop with clear feedback
```bash
local max_retries=12
local retry_interval=5

for i in $(seq 1 $max_retries); do
    if kubectl --context "$cluster_ctx" get crd vteps.k8s.ovn.org &> /dev/null; then
        log_info "✓ VTEP CRD (vteps.k8s.ovn.org) is installed"
        return 0
    fi

    if [ $i -lt $max_retries ]; then
        log_warn "VTEP CRD not found yet, retrying in ${retry_interval}s (attempt $i/$max_retries)..."
        sleep $retry_interval
    fi
done

log_error "VTEP CRD not found after $max_retries attempts!"
return 1
```

**Benefits**:
- Handles kubeconfig timing gracefully (up to 60 seconds)
- Clear progress messages during wait
- Fails cleanly if truly not found

#### 2. FRR-K8s Installation - Kubectl Consistency

**Before**: Mixed kubectl usage
```bash
kubectl --kubeconfig "${SCRIPT_DIR}/kubeconfig-${CLUSTER1_NAME}.yaml" apply -f ...
```

**After**: Consistent context usage
```bash
kubectl --context "kind-${CLUSTER1_NAME}" apply -f ...
kubectl --context "kind-${CLUSTER1_NAME}" wait ... --timeout=3m 2>/dev/null || \
    log_warn "FRR-K8s deployment not ready yet (may need manual check)"
```

**Benefits**:
- Consistent with rest of script
- Longer timeouts (3m instead of 2m)
- Graceful failure with warnings instead of script exit
- No dependency on kubeconfig file export timing

#### 3. Enhanced Output - File Listing

**Before**: Generic success message
```bash
log_info "Setup complete"
```

**After**: Detailed file listing
```bash
log_info "Files created in ${SCRIPT_DIR}:"
log_info "  - kubeconfig-${CLUSTER1_NAME}.yaml"
log_info "  - kubeconfig-${CLUSTER2_NAME}.yaml"
log_info "  - broker-token-${CLUSTER1_NAME}.txt"
log_info "  - broker-token-${CLUSTER2_NAME}.txt"
log_info "  - broker-server.txt"
log_info ""
log_info "Next steps:"
log_info "  1. Build and load agent image:"
log_info "     ./build-and-load.sh"
log_info "  2. Deploy agents:"
log_info "     ./deploy-agents.sh"
```

**Benefits**:
- Users know exactly what was created
- Clear next steps
- Easier debugging (can verify files exist)

#### 4. Script Organization - Deprecation & Purpose

**setup-clusters.sh** (deprecated):
```bash
echo "⚠️  WARNING: This script is DEPRECATED"
echo "⚠️  Please use ./setup-clusters-v2.sh instead"
echo "⚠️  The v2 script clones OVN-K from GitHub and works with CI/CD"
sleep 5
```

**complete-setup.sh** (manual recovery):
```bash
# PURPOSE: This script is for RECOVERY/MANUAL use only if setup-clusters-v2.sh
# fails partway through or if you need to re-run broker/RBAC setup without
# recreating clusters.
#
# NORMAL WORKFLOW: Use ./setup-clusters-v2.sh which does everything automatically.
```

**run-all.sh** (automation):
```bash
# Updated to use v2:
"${SCRIPT_DIR}/setup-clusters-v2.sh"
```

**Benefits**:
- Clear migration path from old to new
- Purpose of each script is obvious
- No confusion about which script to use

### Testing Results

#### Before Updates
```
./setup-clusters-v2.sh
...
[ERROR] VTEP CRD not found!
# Script exits even though VTEP CRD is installed
# Need manual intervention with complete-setup.sh
```

#### After Updates
```
./setup-clusters-v2.sh
...
[WARN] VTEP CRD not found yet, retrying in 5s (attempt 1/12)...
[INFO] ✓ VTEP CRD (vteps.k8s.ovn.org) is installed
[INFO]   VTEP CRD version: v1
...
[INFO] ✓ FRR-K8s installed on cluster1
[INFO] ✓ FRR-K8s installed on cluster2
...
[INFO] === Setup complete ===
[INFO] Files created in /home/yboaron/prj/skynet/test:
[INFO]   - kubeconfig-cluster1.yaml
[INFO]   - kubeconfig-cluster2.yaml
[INFO]   - broker-token-cluster1.txt
[INFO]   - broker-token-cluster2.txt
[INFO]   - broker-server.txt
```

### Files Modified

| File | Change Type | Description |
|------|-------------|-------------|
| `test/setup-clusters-v2.sh` | Enhanced | Added retry logic, fixed kubectl usage, improved output |
| `test/run-all.sh` | Updated | Now uses setup-clusters-v2.sh instead of deprecated script |
| `test/setup-clusters.sh` | Deprecated | Added warning to use v2 script |
| `test/complete-setup.sh` | Documented | Added header explaining manual recovery purpose |

---

## Original Changes - Using OVN-Kubernetes KIND Scripts

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

# Optional: Use pre-built OVN-K image to avoid build issues
export OVN_IMAGE="ghcr.io/ovn-org/ovn-kubernetes/ovn-kube-ubuntu:master"

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
- `OVN_IMAGE` - Use pre-built OVN-K image instead of building from source (default: build from source)
  - Example: `export OVN_IMAGE="ghcr.io/ovn-org/ovn-kubernetes/ovn-kube-ubuntu:master"`

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

### OVN-K image build fails

If the OVN-K build from source fails (e.g., koji package download errors), use a pre-built image:

```bash
export OVN_IMAGE="ghcr.io/ovn-org/ovn-kubernetes/ovn-kube-ubuntu:master"
./setup-clusters.sh
```

This skips the build and uses the official pre-built container image.

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
