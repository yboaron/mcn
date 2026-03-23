#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MANIFESTS_DIR="${PROJECT_ROOT}/test/manifests/skynet-agent"

# Must match kubeconfigs written by setup-clusters-v2.sh (output/kubeconfig-<cluster>.yaml).
cluster_kubeconfig() {
    local name=$1
    echo "${PROJECT_ROOT}/output/kubeconfig-${name}.yaml"
}

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

CLUSTER1_NAME="cluster1"
CLUSTER2_NAME="cluster2"
BROKER_CLUSTER="${BROKER_CLUSTER:-cluster1}"
BROKER_NAMESPACE="skynet-broker"
AGENT_NS="skynet-operator"

# API server URL in skynet-agent-broker Secret (must be reachable from pods on that cluster).
# Broker cluster: in-cluster API. Other clusters: host-published kind port + Docker network gateway.
broker_api_url_for_cluster() {
    local cluster_name=$1
    if [[ "${cluster_name}" == "${BROKER_CLUSTER}" ]]; then
        echo "https://kubernetes.default.svc:443"
        return
    fi
    local kcfg
    kcfg="$(cluster_kubeconfig "${BROKER_CLUSTER}")"
    local raw
    raw=$(kubectl --kubeconfig "${kcfg}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')
    if [[ "${raw}" =~ https://127\.0\.0\.1:([0-9]+) ]]; then
        local port="${BASH_REMATCH[1]}"
        local broker_cp_ip
        broker_cp_ip=$(docker inspect "${BROKER_CLUSTER}-control-plane" -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -n1)
        # Pods on other kind clusters share the Docker "kind" network; apiserver listens on :6443 on the CP node.
        if [[ -n "${broker_cp_ip}" ]]; then
            echo "https://${broker_cp_ip}:${BROKER_API_PORT:-6443}"
            return
        fi
        local host="${BROKER_API_HOST:-}"
        if [[ -z "${host}" ]]; then
            host=$(docker inspect "${BROKER_CLUSTER}-control-plane" -f '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' 2>/dev/null | head -n1)
        fi
        if [[ -z "${host}" ]]; then
            host="172.17.0.1"
        fi
        echo "https://${host}:${port}"
        return
    fi
    echo "${raw}"
}

# Pool defaults must match broker ConfigMap skynet-broker-pools (see setup-clusters-v2.sh).
POOL_ASN_MIN="64512"
POOL_ASN_MAX="65534"
POOL_VTEP_CIDR="100.0.0.0/8"
POOL_VTEP_PREFIX_LEN="16"

# Per-cluster allocation (same order as pkg/operator/alloc on empty broker).
cluster_allocation() {
    local cluster_name=$1
    case "${cluster_name}" in
        cluster1) echo "100.0.0.0/16 64512" ;;
        cluster2) echo "100.1.0.0/16 64513" ;;
        *)
            log_error "Unknown cluster ${cluster_name}; extend cluster_allocation() in deploy-agents.sh"
            exit 1
            ;;
    esac
}

deploy_agent() {
    local cluster_name=$1
    log_info "Deploying SkyNet agent to ${cluster_name} (manifests + Secret/ConfigMap)..."

    local CLUSTER_KUBECONFIG
    CLUSTER_KUBECONFIG="$(cluster_kubeconfig "${cluster_name}")"
    if [[ ! -f "${CLUSTER_KUBECONFIG}" ]]; then
        log_error "Kubeconfig not found: ${CLUSTER_KUBECONFIG}. Run setup-clusters-v2.sh (e.g. make clusters) first."
        exit 1
    fi

    local K="kubectl --kubeconfig ${CLUSTER_KUBECONFIG}"

    BROKER_TOKEN=$(cat "${SCRIPT_DIR}/broker-token-${cluster_name}.txt")
    local BROKER_POD_API_SERVER
    BROKER_POD_API_SERVER="$(broker_api_url_for_cluster "${cluster_name}")"
    log_info "Broker API for agent pods on ${cluster_name}: ${BROKER_POD_API_SERVER}"

    read -r ALLOC_VTEP ALLOC_ASN <<<"$(cluster_allocation "${cluster_name}")"

    log_info "Applying CRDs to ${cluster_name}..."
    ${K} apply -f "${PROJECT_ROOT}/deploy/crds/"

    log_info "Applying static agent manifests (namespace, RBAC, Deployment)..."
    ${K} apply -f "${MANIFESTS_DIR}/namespace.yaml"
    ${K} apply -f "${MANIFESTS_DIR}/rbac.yaml"

    log_info "Syncing broker Secret + agent ConfigMap (${AGENT_NS})..."
    ${K} create secret generic skynet-agent-broker \
        --namespace "${AGENT_NS}" \
        --from-literal=api-server="${BROKER_POD_API_SERVER}" \
        --from-literal=token="${BROKER_TOKEN}" \
        --dry-run=client -o yaml | ${K} apply -f -

    ${K} create configmap skynet-agent-config \
        --namespace "${AGENT_NS}" \
        --from-literal=cluster-id="${cluster_name}" \
        --from-literal=broker-namespace="${BROKER_NAMESPACE}" \
        --from-literal=allocated-vtep-cidr="${ALLOC_VTEP}" \
        --from-literal=allocated-asn="${ALLOC_ASN}" \
        --from-literal=pool-asn-min="${POOL_ASN_MIN}" \
        --from-literal=pool-asn-max="${POOL_ASN_MAX}" \
        --from-literal=pool-vtep-cidr="${POOL_VTEP_CIDR}" \
        --from-literal=pool-vtep-prefix-len="${POOL_VTEP_PREFIX_LEN}" \
        --dry-run=client -o yaml | ${K} apply -f -

    ${K} apply -f "${MANIFESTS_DIR}/deployment.yaml"

    log_info "Waiting for skynet-agent rollout on ${cluster_name}..."
    if ! ${K} rollout status deployment/skynet-agent -n "${AGENT_NS}" --timeout=180s; then
        log_error "Rollout failed on ${cluster_name}. Logs: kubectl --kubeconfig ${CLUSTER_KUBECONFIG} -n ${AGENT_NS} logs -l app=skynet-agent --tail=80"
        exit 1
    fi

    log_info "Agent ready on ${cluster_name}"
}

main() {
    log_info "=== Deploying SkyNet agents (kubectl apply -f manifests) ==="

    if [[ ! -d "${MANIFESTS_DIR}" ]]; then
        log_error "Missing ${MANIFESTS_DIR}"
        exit 1
    fi

    if [[ ! -f "${SCRIPT_DIR}/broker-token-${CLUSTER1_NAME}.txt" ]] || [[ ! -f "${SCRIPT_DIR}/broker-token-${CLUSTER2_NAME}.txt" ]]; then
        log_error "Broker tokens not found. Run setup-clusters-v2.sh (e.g. make clusters) first."
        exit 1
    fi

    deploy_agent "${CLUSTER1_NAME}"
    deploy_agent "${CLUSTER2_NAME}"

    log_info "=== Deployment complete ==="
    echo ""
    log_info "Broker Cluster CRs (synced by agents):"
    echo "  kubectl --kubeconfig $(cluster_kubeconfig "${CLUSTER1_NAME}") -n ${BROKER_NAMESPACE} get clusters -o wide"
    echo ""
    log_info "Verify full mesh (VTEP, FRRConfiguration, BGP checks):"
    echo "  make verify-bgp"
    echo ""
    log_info "Agent logs:"
    echo "  kubectl --kubeconfig $(cluster_kubeconfig "${CLUSTER1_NAME}") -n ${AGENT_NS} logs -l app=skynet-agent -f"
}

main "$@"
