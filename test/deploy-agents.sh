#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
KUBECONFIG_DIR="${PROJECT_ROOT}/output/kubeconfigs"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
CLUSTER1_NAME="cluster1"
CLUSTER2_NAME="cluster2"
BROKER_NAMESPACE="skynet-broker"
AGENT_IMAGE="skynet-agent:latest"

# Deploy agent to a cluster
deploy_agent() {
    local cluster_name=$1
    log_info "Deploying SkyNet agent to ${cluster_name}..."

    local CLUSTER_KUBECONFIG="${KUBECONFIG_DIR}/kind-config-${cluster_name}"

    # Read broker configuration
    BROKER_SERVER=$(cat "${SCRIPT_DIR}/broker-server.txt")
    BROKER_TOKEN=$(cat "${SCRIPT_DIR}/broker-token-${cluster_name}.txt")

    # Apply CRDs to local cluster
    log_info "Applying CRDs to ${cluster_name}..."
    kubectl --kubeconfig "${CLUSTER_KUBECONFIG}" apply -f "${PROJECT_ROOT}/deploy/crds/"

    # Create agent deployment
    cat <<EOF | kubectl --kubeconfig "${CLUSTER_KUBECONFIG}" apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: skynet-agent
  namespace: skynet-operator
  labels:
    app: skynet-agent
spec:
  replicas: 1
  selector:
    matchLabels:
      app: skynet-agent
  template:
    metadata:
      labels:
        app: skynet-agent
    spec:
      serviceAccountName: skynet-agent
      containers:
      - name: agent
        image: ${AGENT_IMAGE}
        imagePullPolicy: Never
        args:
        - --cluster-id=${cluster_name}
        - --broker-server=${BROKER_SERVER}
        - --broker-token=${BROKER_TOKEN}
        - --broker-namespace=${BROKER_NAMESPACE}
        - --v=4
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
EOF

    log_info "Agent deployed to ${cluster_name}"
}

# Main
main() {
    log_info "=== Deploying SkyNet agents ==="

    # Check if broker config exists
    if [[ ! -f "${SCRIPT_DIR}/broker-server.txt" ]] || [[ ! -f "${SCRIPT_DIR}/broker-token-${CLUSTER1_NAME}.txt" ]]; then
        log_error "Broker configuration not found. Please run setup-clusters.sh first."
        exit 1
    fi

    deploy_agent "${CLUSTER1_NAME}"
    deploy_agent "${CLUSTER2_NAME}"

    log_info "=== Deployment complete ==="
    log_info ""
    log_info "Check agent status:"
    log_info "  kubectl --kubeconfig ${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME} -n skynet-operator get pods"
    log_info "  kubectl --kubeconfig ${KUBECONFIG_DIR}/kind-config-${CLUSTER2_NAME} -n skynet-operator get pods"
    log_info ""
    log_info "Check agent logs:"
    log_info "  kubectl --kubeconfig ${KUBECONFIG_DIR}/kind-config-${CLUSTER1_NAME} -n skynet-operator logs -l app=skynet-agent -f"
}

main "$@"
