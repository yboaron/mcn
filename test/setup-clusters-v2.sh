#!/bin/bash

# SkyNet Test Environment Setup
# Clones OVN-K from GitHub (like Shipyard does) instead of using local repo

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== Setting up SkyNet test environment (Shipyard-style) ==="

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

# OVN-K Configuration (like Shipyard)
OVNK_REPO="${OVNK_REPO:-https://github.com/ovn-org/ovn-kubernetes.git}"
OVNK_BRANCH="${OVNK_BRANCH:-master}"  # or specific tag like "v0.4.0"
OVNK_CLONE_DIR="/tmp/ovn-kubernetes-skynet-$$"

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function
cleanup_ovnk_clone() {
    if [ -d "$OVNK_CLONE_DIR" ]; then
        log_info "Cleaning up OVN-K clone..."
        rm -rf "$OVNK_CLONE_DIR"
    fi
}

# Register cleanup on exit
trap cleanup_ovnk_clone EXIT

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

    log_info "All prerequisites met"
}

# Clone OVN-K from GitHub (Shipyard approach)
clone_ovnk() {
    log_info "Cloning OVN-Kubernetes from GitHub..."
    log_info "  Repo: $OVNK_REPO"
    log_info "  Branch: $OVNK_BRANCH"
    log_info "  Clone dir: $OVNK_CLONE_DIR"

    git clone --depth 1 --branch "$OVNK_BRANCH" "$OVNK_REPO" "$OVNK_CLONE_DIR"

    if [ ! -f "$OVNK_CLONE_DIR/contrib/kind.sh" ]; then
        log_error "kind.sh not found in cloned OVN-K repo"
        exit 1
    fi

    log_info "✓ OVN-Kubernetes cloned successfully"
}

# Patch kind-common.sh to use ubuntu-image instead of fedora-image (avoids koji package issues)
patch_kind_for_ubuntu() {
    local KIND_COMMON="$OVNK_CLONE_DIR/contrib/kind-common.sh"

    if grep -q "fedora-image" "$KIND_COMMON"; then
        log_info "Patching kind-common.sh to use ubuntu-image..."
        sed -i.bak 's/fedora-image/ubuntu-image/g' "$KIND_COMMON"
        log_info "✓ Patched to use ubuntu-image"
    fi
}

# Create KIND clusters using OVN-K kind.sh
create_clusters() {
    log_info "Creating KIND clusters with OVN-Kubernetes..."
    log_info "Building OVN-K from source for latest VTEP CRD support"

    local KIND_SH="$OVNK_CLONE_DIR/contrib/kind.sh"

    # Patch to use ubuntu-image
    patch_kind_for_ubuntu

    # Create cluster1
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER1_NAME}$"; then
        log_warn "Cluster ${CLUSTER1_NAME} already exists, skipping creation"
    else
        log_info "Creating cluster ${CLUSTER1_NAME} with $NUM_WORKERS workers (this may take 5-10 minutes)..."

        pushd "$OVNK_CLONE_DIR" > /dev/null
        KIND_CLUSTER_NAME="$CLUSTER1_NAME" \
        KIND_NUM_WORKER="$NUM_WORKERS" \
          "$KIND_SH" -wk "$NUM_WORKERS" -ic
        popd > /dev/null

        # Export kubeconfig
        kind export kubeconfig --name "$CLUSTER1_NAME" --kubeconfig "${SCRIPT_DIR}/kubeconfig-${CLUSTER1_NAME}.yaml"
        log_info "✓ Cluster ${CLUSTER1_NAME} created"
    fi

    # Create cluster2
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER2_NAME}$"; then
        log_warn "Cluster ${CLUSTER2_NAME} already exists, skipping creation"
    else
        log_info "Creating cluster ${CLUSTER2_NAME} with $NUM_WORKERS workers (this may take 5-10 minutes)..."

        pushd "$OVNK_CLONE_DIR" > /dev/null
        KIND_CLUSTER_NAME="$CLUSTER2_NAME" \
        KIND_NUM_WORKER="$NUM_WORKERS" \
          "$KIND_SH" -wk "$NUM_WORKERS" -ic
        popd > /dev/null

        # Export kubeconfig
        kind export kubeconfig --name "$CLUSTER2_NAME" --kubeconfig "${SCRIPT_DIR}/kubeconfig-${CLUSTER2_NAME}.yaml"
        log_info "✓ Cluster ${CLUSTER2_NAME} created"
    fi

    log_info "KIND clusters with OVN-Kubernetes created successfully"
}

# Kubeconfig for a kind cluster (stored in output/ at project root).
cluster_kubeconfig_path() {
    local name=$1
    echo "${PROJECT_ROOT}/output/kubeconfig-${name}.yaml"
}

ensure_cluster_kubeconfig() {
    local name=$1
    if ! kind get clusters 2>/dev/null | grep -q "^${name}$"; then
        log_error "Kind cluster ${name} not found; cannot write kubeconfig"
        return 1
    fi
    # Ensure output directory exists
    mkdir -p "${PROJECT_ROOT}/output"
    kind get kubeconfig --name "$name" >"$(cluster_kubeconfig_path "$name")"
}

# kubectl using repo-local kubeconfig (avoids stale ~/.kube/config for kind-cluster* contexts).
kubectl_kind() {
    local name=$1
    shift
    kubectl --kubeconfig "$(cluster_kubeconfig_path "$name")" "$@"
}

# Verify VTEP CRD is installed
verify_vtep_crd() {
    log_info "Verifying VTEP CRD is installed..."

    ensure_cluster_kubeconfig "${CLUSTER1_NAME}"
    local kcfg
    kcfg="$(cluster_kubeconfig_path "${CLUSTER1_NAME}")"
    local max_retries=12
    local retry_interval=5

    for i in $(seq 1 $max_retries); do
        if kubectl --kubeconfig "$kcfg" get crd vteps.k8s.ovn.org &> /dev/null; then
            log_info "✓ VTEP CRD (vteps.k8s.ovn.org) is installed"

            local crd_version
            crd_version=$(kubectl --kubeconfig "$kcfg" get crd vteps.k8s.ovn.org -o jsonpath='{.spec.versions[0].name}')
            log_info "  VTEP CRD version: $crd_version"
            return 0
        fi

        if [ "$i" -lt $max_retries ]; then
            log_warn "VTEP CRD not found yet, retrying in ${retry_interval}s (attempt $i/$max_retries)..."
            sleep $retry_interval
        fi
    done

    log_error "VTEP CRD not found after $max_retries attempts!"
    log_error "OVN-K version may not support VTEP."
    log_error "Try using a newer OVN-K branch or master."
    return 1
}

# Install FRR-K8s for BGP support
install_frr_k8s_all() {
    log_info "Installing FRR-K8s on both clusters..."

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

    ensure_cluster_kubeconfig "${CLUSTER1_NAME}"
    ensure_cluster_kubeconfig "${CLUSTER2_NAME}"

    # Install on cluster1
    log_info "Installing FRR-K8s on ${CLUSTER1_NAME}..."
    kubectl_kind "${CLUSTER1_NAME}" apply -f "${FRR_TMP_DIR}/frr-k8s/config/all-in-one/frr-k8s.yaml"

    log_info "Waiting for FRR-K8s to be ready on ${CLUSTER1_NAME}..."
    kubectl_kind "${CLUSTER1_NAME}" wait -n frr-k8s-system deployment frr-k8s-statuscleaner --for condition=Available --timeout=3m 2>/dev/null || log_warn "FRR-K8s deployment not ready yet (may need manual check)"
    kubectl_kind "${CLUSTER1_NAME}" rollout status -n frr-k8s-system daemonset frr-k8s-daemon --timeout=3m 2>/dev/null || log_warn "FRR-K8s daemonset not ready yet (may need manual check)"

    log_info "✓ FRR-K8s installed on ${CLUSTER1_NAME}"

    # Install on cluster2
    log_info "Installing FRR-K8s on ${CLUSTER2_NAME}..."
    kubectl_kind "${CLUSTER2_NAME}" apply -f "${FRR_TMP_DIR}/frr-k8s/config/all-in-one/frr-k8s.yaml"

    log_info "Waiting for FRR-K8s to be ready on ${CLUSTER2_NAME}..."
    kubectl_kind "${CLUSTER2_NAME}" wait -n frr-k8s-system deployment frr-k8s-statuscleaner --for condition=Available --timeout=3m 2>/dev/null || log_warn "FRR-K8s deployment not ready yet (may need manual check)"
    kubectl_kind "${CLUSTER2_NAME}" rollout status -n frr-k8s-system daemonset frr-k8s-daemon --timeout=3m 2>/dev/null || log_warn "FRR-K8s daemonset not ready yet (may need manual check)"

    log_info "✓ FRR-K8s installed on ${CLUSTER2_NAME}"
}

# Setup broker cluster
setup_broker() {
    log_info "Setting up broker on ${BROKER_CLUSTER}..."

    ensure_cluster_kubeconfig "${BROKER_CLUSTER}"

    # Create broker namespace
    kubectl_kind "${BROKER_CLUSTER}" create namespace "${BROKER_NAMESPACE}" --dry-run=client -o yaml | kubectl_kind "${BROKER_CLUSTER}" apply -f -

    # Pool bounds for skynet-operator allocators (see pkg/operator/alloc/poolconfig.go)
    kubectl_kind "${BROKER_CLUSTER}" apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: skynet-broker-pools
  namespace: ${BROKER_NAMESPACE}
data:
  asnMin: "64512"
  asnMax: "65534"
  vtepPoolCIDR: "100.0.0.0/8"
  vtepPrefixLen: "16"
EOF

    # Apply SkyNet CRDs
    log_info "Applying SkyNet CRDs to broker..."
    kubectl_kind "${BROKER_CLUSTER}" apply -f "${PROJECT_ROOT}/deploy/crds/"

    # Wait for CRDs to be established
    log_info "Waiting for CRDs to be established..."
    kubectl_kind "${BROKER_CLUSTER}" wait --for condition=established --timeout=60s \
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

    ensure_cluster_kubeconfig "${cluster_name}"

    cat <<EOF | kubectl_kind "${cluster_name}" apply -f -
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

    ensure_cluster_kubeconfig "${BROKER_CLUSTER}"

    # Create service account for the agent on broker
    cat <<EOF | kubectl_kind "${BROKER_CLUSTER}" apply -f -
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
    kubectl_kind "${BROKER_CLUSTER}" create token "skynet-agent-${cluster_name}" \
        -n "${BROKER_NAMESPACE}" \
        --duration=87600h > "${SCRIPT_DIR}/broker-token-${cluster_name}.txt"

    # API URL for kubectl from the *host* (127.0.0.1:<port> is fine). Agent pods use per-cluster URLs in deploy-agents.sh.
    BROKER_SERVER=$(kubectl --kubeconfig "$(cluster_kubeconfig_path "${BROKER_CLUSTER}")" config view --minify -o jsonpath='{.clusters[0].cluster.server}')
    echo "${BROKER_SERVER}" > "${SCRIPT_DIR}/broker-server.txt"

    log_info "Broker access created for ${cluster_name}"
}

# Save kubeconfig contexts
save_kubeconfigs() {
    log_info "Saving kubeconfigs..."

    # Export cluster1 kubeconfig
    kind get kubeconfig --name "${CLUSTER1_NAME}" > "${SCRIPT_DIR}/kubeconfig-${CLUSTER1_NAME}.yaml"

    # Export cluster2 kubeconfig
    kind get kubeconfig --name "${CLUSTER2_NAME}" > "${SCRIPT_DIR}/kubeconfig-${CLUSTER2_NAME}.yaml"

    log_info "Kubeconfigs saved to ${SCRIPT_DIR}"
}

# Main setup flow
main() {
    log_info "Configuration:"
    log_info "  OVN-K Repo: $OVNK_REPO"
    log_info "  OVN-K Branch: $OVNK_BRANCH"
    log_info "  Cluster 1: $CLUSTER1_NAME"
    log_info "  Cluster 2: $CLUSTER2_NAME"
    log_info "  Workers per cluster: $NUM_WORKERS"
    log_info "  Broker: $BROKER_CLUSTER"
    log_info ""

    check_prerequisites
    clone_ovnk
    create_clusters
    verify_vtep_crd

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
    log_info "  ✓ OVN-Kubernetes CNI (built from ${OVNK_BRANCH})"
    log_info "  ✓ VTEP CRD support verified"
    log_info "  ✓ FRR-K8s for BGP support"
    log_info "  ✓ SkyNet CRDs on broker"
    log_info "  ✓ RBAC configured for agents"
    log_info "  ✓ Broker tokens created"
    log_info ""
    log_info "OVN-K clone will be cleaned up automatically"
    log_info ""
    log_info "Files created:"
    log_info "  - ${PROJECT_ROOT}/output/kubeconfig-${CLUSTER1_NAME}.yaml"
    log_info "  - ${PROJECT_ROOT}/output/kubeconfig-${CLUSTER2_NAME}.yaml"
    log_info "  - ${SCRIPT_DIR}/broker-token-${CLUSTER1_NAME}.txt"
    log_info "  - ${SCRIPT_DIR}/broker-token-${CLUSTER2_NAME}.txt"
    log_info "  - ${SCRIPT_DIR}/broker-server.txt"
    log_info ""
    log_info "Next steps:"
    log_info "  1. Build and load agent image:"
    log_info "     ./build-and-load.sh"
    log_info "  2. Deploy agents:"
    log_info "     ./deploy-agents.sh"
    log_info "  3. Verify BGP setup:"
    log_info "     ./verify-bgp.sh"
    log_info ""
    log_info "Cluster contexts:"
    log_info "  Cluster 1: kind-${CLUSTER1_NAME}"
    log_info "  Cluster 2: kind-${CLUSTER2_NAME}"
    log_info "  Broker: ${BROKER_CLUSTER} (namespace: ${BROKER_NAMESPACE})"
}

main "$@"
