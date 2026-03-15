#!/bin/bash

# DEPRECATED: This script uses a local OVN-K repo path and is not suitable for CI/CD.
# Please use setup-clusters-v2.sh instead, which clones OVN-K from GitHub.
#
# This script is kept for backward compatibility only.

echo ""
echo "⚠️  WARNING: This script is DEPRECATED"
echo "⚠️  Please use ./setup-clusters-v2.sh instead"
echo "⚠️  The v2 script clones OVN-K from GitHub and works with CI/CD"
echo ""
echo "Press Ctrl+C within 5 seconds to cancel, or wait to continue with deprecated script..."
sleep 5

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
KUBECONFIG_DIR="${PROJECT_ROOT}/output/kubeconfigs"

echo "=== Setting up SkyNet test environment (DEPRECATED) ==="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER1_NAME="${CLUSTER1_NAME:-cluster1}"
CLUSTER2_NAME="${CLUSTER2_NAME:-cluster2}"
BROKER_CLUSTER="${CLUSTER1_NAME}"
BROKER_NAMESPACE="skynet-broker"
NUM_WORKERS="${NUM_WORKERS:-2}"
OVNK_REPO_PATH="${OVNK_REPO_PATH:-/home/yboaron/prj/ovn-kubernetes}"
# OVN_IMAGE - Optional: specify source image (default: ghcr.io/ovn-org/ovn-kubernetes/ovn-kube-ubuntu:master)
# Will be pulled, tagged, and pushed to local registry at localhost:5000/ovn-kube:latest

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v kind &> /dev/null; then
        log_error "kind not found. Please install kind: https://kind.sigs.k8s.io/docs/user/quick-start/"
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl"
        exit 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "docker not found. Please install docker"
        exit 1
    fi

    if ! command -v git &> /dev/null; then
        log_error "git not found. Please install git"
        exit 1
    fi

    # Check for OVN-K repo
    if [ ! -d "$OVNK_REPO_PATH" ]; then
        log_error "OVN-Kubernetes repo not found at: $OVNK_REPO_PATH"
        log_error "Please set OVNK_REPO_PATH environment variable or clone the repo:"
        log_error "  git clone https://github.com/ovn-org/ovn-kubernetes $OVNK_REPO_PATH"
        exit 1
    fi

    if [ ! -f "$OVNK_REPO_PATH/contrib/kind.sh" ]; then
        log_error "kind.sh not found in OVN-K repo at: $OVNK_REPO_PATH/contrib/kind.sh"
        exit 1
    fi

    log_info "All prerequisites met"
    log_info "Using OVN-K repo: $OVNK_REPO_PATH"
}

# Patch kind-common.sh to use ubuntu-image instead of fedora-image (avoids koji package issues)
patch_kind_for_ubuntu() {
    local KIND_COMMON="$OVNK_REPO_PATH/contrib/kind-common.sh"

    if grep -q "fedora-image" "$KIND_COMMON"; then
        log_info "Patching kind-common.sh to use ubuntu-image instead of fedora-image..."
        sed -i.bak 's/fedora-image/ubuntu-image/g' "$KIND_COMMON"
        log_info "✓ Patched to use ubuntu-image"
    fi
}

# Restore kind-common.sh after cluster creation
restore_kind_common() {
    local KIND_COMMON="$OVNK_REPO_PATH/contrib/kind-common.sh"

    if [ -f "${KIND_COMMON}.bak" ]; then
        log_info "Restoring original kind-common.sh..."
        mv "${KIND_COMMON}.bak" "$KIND_COMMON"
    fi
}

# Create KIND clusters using OVN-K kind.sh (build from source for version compatibility)
create_clusters() {
    log_info "Creating KIND clusters with OVN-Kubernetes..."
    log_info "Building OVN-K from source in ${OVNK_REPO_PATH} for version compatibility"
    log_info "Using Ubuntu-based image to avoid koji package issues"

    local KIND_SH="$OVNK_REPO_PATH/contrib/kind.sh"

    # Patch to use ubuntu-image
    patch_kind_for_ubuntu

    # Create cluster1
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER1_NAME}$"; then
        log_warn "Cluster ${CLUSTER1_NAME} already exists, skipping creation"
    else
        log_info "Creating cluster ${CLUSTER1_NAME} with $NUM_WORKERS workers (this may take 5-10 minutes)..."

        pushd "$OVNK_REPO_PATH" > /dev/null
        KIND_CLUSTER_NAME="$CLUSTER1_NAME" \
        KIND_NUM_WORKER="$NUM_WORKERS" \
          "$KIND_SH" -wk "$NUM_WORKERS" -ic
        popd > /dev/null

        # Export kubeconfig
        kind export kubeconfig --name "$CLUSTER1_NAME" --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}"
        log_info "✓ Cluster ${CLUSTER1_NAME} created with OVN-Kubernetes"
    fi

    # Create cluster2
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER2_NAME}$"; then
        log_warn "Cluster ${CLUSTER2_NAME} already exists, skipping creation"
    else
        log_info "Creating cluster ${CLUSTER2_NAME} with $NUM_WORKERS workers (this may take 5-10 minutes)..."

        pushd "$OVNK_REPO_PATH" > /dev/null
        KIND_CLUSTER_NAME="$CLUSTER2_NAME" \
        KIND_NUM_WORKER="$NUM_WORKERS" \
          "$KIND_SH" -wk "$NUM_WORKERS" -ic
        popd > /dev/null

        # Export kubeconfig
        kind export kubeconfig --name "$CLUSTER2_NAME" --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}"
        log_info "✓ Cluster ${CLUSTER2_NAME} created with OVN-Kubernetes"
    fi

    # Restore original kind-common.sh
    restore_kind_common

    log_info "KIND clusters with OVN-Kubernetes created successfully"
}

# Install FRR-K8s for BGP support
install_frr_k8s_all() {
    log_info "Installing FRR-K8s on both clusters..."

    # Based on: https://github.com/yboaron/ovn-bgp-mcn-udn-poc/blob/main/scripts/0a-install-frr-k8s.sh
    FRR_K8S_VERSION="${FRR_K8S_VERSION:-v0.0.21}"

    # Create temp directory
    FRR_TMP_DIR=$(mktemp -d)
    trap 'rm -rf $FRR_TMP_DIR' EXIT

    log_info "Cloning FRR-k8s repository (${FRR_K8S_VERSION})..."
    pushd "$FRR_TMP_DIR" > /dev/null
    git clone --depth 1 --branch $FRR_K8S_VERSION https://github.com/metallb/frr-k8s

    # Download and apply OVN-K patches
    log_info "Downloading and applying OVN-K patches..."
    curl -Ls https://github.com/jcaamano/frr-k8s/archive/refs/heads/ovnk-bgp-v0.0.21.tar.gz | \
        tar xzvf - frr-k8s-ovnk-bgp-v0.0.21/patches --strip-components 1

    pushd frr-k8s > /dev/null
    git apply ../patches/* || log_warn "Failed to apply some patches (may be expected)"
    popd > /dev/null
    popd > /dev/null

    # Install on cluster1
    log_info "Installing FRR-K8s on ${CLUSTER1_NAME}..."
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}" apply -f "${FRR_TMP_DIR}/frr-k8s/config/all-in-one/frr-k8s.yaml"

    log_info "Waiting for FRR-K8s to be ready on ${CLUSTER1_NAME}..."
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}" wait -n frr-k8s-system deployment frr-k8s-statuscleaner --for condition=Available --timeout=2m || true
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}" rollout status -n frr-k8s-system daemonset frr-k8s-daemon --timeout=2m || true

    log_info "✓ FRR-K8s installed on ${CLUSTER1_NAME}"

    # Install on cluster2
    log_info "Installing FRR-K8s on ${CLUSTER2_NAME}..."
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}" apply -f "${FRR_TMP_DIR}/frr-k8s/config/all-in-one/frr-k8s.yaml"

    log_info "Waiting for FRR-K8s to be ready on ${CLUSTER2_NAME}..."
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}" wait -n frr-k8s-system deployment frr-k8s-statuscleaner --for condition=Available --timeout=2m || true
    kubectl --kubeconfig "${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}" rollout status -n frr-k8s-system daemonset frr-k8s-daemon --timeout=2m || true

    log_info "✓ FRR-K8s installed on ${CLUSTER2_NAME}"
}

# Setup broker cluster
setup_broker() {
    log_info "Setting up broker on ${BROKER_CLUSTER}..."

    kubectl config use-context "kind-${BROKER_CLUSTER}"

    # Create broker namespace
    kubectl create namespace "${BROKER_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

    # Apply SkyNet CRDs
    log_info "Applying SkyNet CRDs to broker..."
    kubectl apply -f "${PROJECT_ROOT}/deploy/crds/"

    # Wait for CRDs to be established
    log_info "Waiting for CRDs to be established..."
    kubectl wait --for condition=established --timeout=60s \
        crd/clusters.skynet.io \
        crd/multiclusternetworks.skynet.io \
        crd/multiclusternetworkconnects.skynet.io \
        crd/skynets.skynet.io

    log_info "Broker setup complete"
}

# Create service account and RBAC for agent
create_agent_rbac() {
    local cluster_name=$1
    log_info "Creating agent RBAC for ${cluster_name}..."

    kubectl config use-context "kind-${cluster_name}"

    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: Namespace
metadata:
  name: skynet-operator
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skynet-agent
  namespace: skynet-operator
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: skynet-agent
rules:
- apiGroups: [""]
  resources: ["nodes", "namespaces"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["skynet.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["k8s.ovn.org"]
  resources: ["vteps", "userdefinednetworks"]
  verbs: ["*"]
- apiGroups: ["frrk8s.metallb.io"]
  resources: ["frrconfigurations"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: skynet-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: skynet-agent
subjects:
- kind: ServiceAccount
  name: skynet-agent
  namespace: skynet-operator
EOF

    log_info "Agent RBAC created for ${cluster_name}"
}

# Create broker access for agent
create_broker_access() {
    local cluster_name=$1
    log_info "Creating broker access for ${cluster_name}..."

    # Switch to broker cluster
    kubectl config use-context "kind-${BROKER_CLUSTER}"

    # Create service account for the agent on broker
    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skynet-agent-${cluster_name}
  namespace: ${BROKER_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: skynet-agent-${cluster_name}
  namespace: ${BROKER_NAMESPACE}
rules:
- apiGroups: ["skynet.io"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: skynet-agent-${cluster_name}
  namespace: ${BROKER_NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: skynet-agent-${cluster_name}
subjects:
- kind: ServiceAccount
  name: skynet-agent-${cluster_name}
  namespace: ${BROKER_NAMESPACE}
EOF

    # Create token for the service account
    log_info "Creating token for ${cluster_name} agent..."
    kubectl create token "skynet-agent-${cluster_name}" \
        -n "${BROKER_NAMESPACE}" \
        --duration=87600h > "${SCRIPT_DIR}/broker-token-${cluster_name}.txt"

    # Get broker API server
    BROKER_SERVER=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='kind-${BROKER_CLUSTER}')].cluster.server}")
    echo "${BROKER_SERVER}" > "${SCRIPT_DIR}/broker-server.txt"

    log_info "Broker access created for ${cluster_name}"
}

# Save kubeconfig contexts
save_kubeconfigs() {
    log_info "Saving kubeconfigs..."

    # Create output directory (Submariner-style)
    mkdir -p "${KUBECONFIG_DIR}"

    # Export cluster1 kubeconfig
    kind get kubeconfig --name "${CLUSTER1_NAME}" > "${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}"

    # Export cluster2 kubeconfig
    kind get kubeconfig --name "${CLUSTER2_NAME}" > "${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}"

    log_info "Kubeconfigs saved to ${KUBECONFIG_DIR}"
}

# Main setup flow
main() {
    log_info "Configuration:"
    log_info "  OVN-K Repo: $OVNK_REPO_PATH"
    log_info "  Cluster 1: $CLUSTER1_NAME"
    log_info "  Cluster 2: $CLUSTER2_NAME"
    log_info "  Workers per cluster: $NUM_WORKERS"
    log_info "  Broker: $BROKER_CLUSTER"
    log_info ""

    check_prerequisites
    create_clusters

    # Install FRR-K8s on both clusters
    log_info ""
    log_info "=== Installing FRR-K8s ==="
    install_frr_k8s_all

    # Wait for clusters to stabilize
    log_info "Waiting 30 seconds for clusters to stabilize..."
    sleep 30

    # Setup broker and RBAC
    log_info ""
    log_info "=== Setting up Broker ==="
    setup_broker
    create_agent_rbac "${CLUSTER1_NAME}"
    create_agent_rbac "${CLUSTER2_NAME}"
    create_broker_access "${CLUSTER1_NAME}"
    create_broker_access "${CLUSTER2_NAME}"
    save_kubeconfigs

    log_info ""
    log_info "=== Setup complete ==="
    log_info ""
    log_info "Clusters created with:"
    log_info "  ✓ OVN-Kubernetes CNI (with interconnect)"
    log_info "  ✓ FRR-K8s for BGP support"
    log_info "  ✓ SkyNet CRDs on broker"
    log_info "  ✓ RBAC configured"
    log_info ""
    log_info "Next steps:"
    log_info "  1. Build and load agent image:"
    log_info "     cd test && ./build-and-load.sh"
    log_info "  2. Deploy agents:"
    log_info "     cd test && ./deploy-agents.sh"
    log_info "  3. Verify setup:"
    log_info "     cd test && ./verify-setup.sh"
    log_info ""
    log_info "Or run complete setup:"
    log_info "  cd test && ./run-all.sh"
    log_info ""
    log_info "Cluster contexts:"
    log_info "  Cluster 1: kind-${CLUSTER1_NAME}"
    log_info "  Cluster 2: kind-${CLUSTER2_NAME}"
    log_info "  Broker: kind-${BROKER_CLUSTER}"
    log_info ""
    log_info "Kubeconfigs saved:"
    log_info "  ${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME}"
    log_info "  ${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME}"
}

main "$@"
