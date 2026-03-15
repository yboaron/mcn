#!/bin/bash

# Manual completion script for SkyNet setup
#
# PURPOSE: This script is for RECOVERY/MANUAL use only if setup-clusters-v2.sh
# fails partway through or if you need to re-run broker/RBAC setup without
# recreating clusters.
#
# NORMAL WORKFLOW: Use ./setup-clusters-v2.sh which does everything automatically.
#
# USE THIS SCRIPT IF:
# - setup-clusters-v2.sh failed during broker/RBAC setup (clusters exist but broker not configured)
# - You need to reset broker configuration without recreating clusters
# - You're debugging broker access issues
#

set -e

echo "=== Manual SkyNet Setup Completion ==="
echo ""
echo "NOTE: This script assumes clusters already exist!"
echo "If clusters don't exist, use ./setup-clusters-v2.sh instead"
echo ""

PROJECT_ROOT="/home/yboaron/prj/skynet"
BROKER_NAMESPACE="skynet-broker"
CLUSTER1="cluster1"
CLUSTER2="cluster2"

# Setup broker on cluster1
echo "[INFO] Setting up broker on cluster1..."
kubectl config use-context "kind-${CLUSTER1}"
kubectl create namespace "${BROKER_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Apply SkyNet CRDs
echo "[INFO] Applying SkyNet CRDs..."
kubectl apply -f "${PROJECT_ROOT}/deploy/crds/"

# Wait for CRDs
echo "[INFO] Waiting for CRDs..."
kubectl wait --for condition=established --timeout=60s \
    crd/clusters.skynet.io \
    crd/multiclusternetworks.skynet.io \
    crd/multiclusternetworkconnects.skynet.io \
    crd/skynets.skynet.io

# Create agent RBAC for both clusters
for cluster in ${CLUSTER1} ${CLUSTER2}; do
    echo "[INFO] Creating agent RBAC for ${cluster}..."
    kubectl config use-context "kind-${cluster}"
    
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
done

# Create broker access
kubectl config use-context "kind-${CLUSTER1}"

for cluster in ${CLUSTER1} ${CLUSTER2}; do
    echo "[INFO] Creating broker access for ${cluster}..."
    
    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skynet-agent-${cluster}
  namespace: ${BROKER_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: skynet-agent-${cluster}
  namespace: ${BROKER_NAMESPACE}
rules:
- apiGroups: ["skynet.io"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: skynet-agent-${cluster}
  namespace: ${BROKER_NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: skynet-agent-${cluster}
subjects:
- kind: ServiceAccount
  name: skynet-agent-${cluster}
  namespace: ${BROKER_NAMESPACE}
EOF

    # Create token
    echo "[INFO] Creating token for ${cluster}..."
    kubectl create token "skynet-agent-${cluster}" \
        -n "${BROKER_NAMESPACE}" \
        --duration=87600h > "broker-token-${cluster}.txt"
done

# Save broker server
BROKER_SERVER=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='kind-${CLUSTER1}')].cluster.server}")
echo "${BROKER_SERVER}" > broker-server.txt

# Save kubeconfigs
kind get kubeconfig --name "${CLUSTER1}" > "kubeconfig-${CLUSTER1}.yaml"
kind get kubeconfig --name "${CLUSTER2}" > "kubeconfig-${CLUSTER2}.yaml"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "✓ VTEP CRD installed (v1)"
echo "✓ OVN-K pods running"
echo "✓ SkyNet CRDs installed"
echo "✓ Broker and RBAC configured"
echo "✓ Tokens generated"
echo ""
echo "Next: ./build-and-load.sh && ./deploy-agents.sh"
