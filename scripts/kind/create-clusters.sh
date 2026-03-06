#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Copyright Contributors to the Submariner project.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Creates MCN KIND clusters with OVN-Kubernetes CNI in Interconnect (IC) mode.
# Uses ovn-kubernetes/contrib/kind.sh — the same approach as Submariner.
# Safe to re-run — existing clusters are skipped.
#
# Usage:
#   ./scripts/kind/create-clusters.sh
#
# Environment overrides:
#   CLUSTERS        space-separated list of cluster names  (default: "cluster1 cluster2")
#   OVN_K_REPO     git repo to clone OVN-K from            (default: upstream GitHub)
#   OVN_K_BRANCH   branch/tag to clone                     (default: master)
#   OVN_K_DIR      where to clone OVN-K                    (default: /opt/ovn-kubernetes)
#   OVN_IMAGE      OVN-K container image to use            (default: ghcr.io upstream master)
#   WORKERS        number of worker nodes per cluster      (default: 2)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=scripts/lib/utils.sh
source "${SCRIPT_DIR}/../lib/utils.sh"

CLUSTERS="${CLUSTERS:-cluster1 cluster2}"
OVN_K_REPO="${OVN_K_REPO:-https://github.com/ovn-kubernetes/ovn-kubernetes.git}"
OVN_K_BRANCH="${OVN_K_BRANCH:-master}"
OVN_K_DIR="${OVN_K_DIR:-/opt/ovn-kubernetes}"
OVN_IMAGE="${OVN_IMAGE:-ghcr.io/ovn-kubernetes/ovn-kubernetes/ovn-kube-ubuntu:master}"
WORKERS="${WORKERS:-2}"

# Pod and service CIDRs per cluster — non-overlapping across clusters
declare -A POD_CIDR=(
    [cluster1]="10.0.0.0/16/24"
    [cluster2]="10.2.0.0/16/24"
)
declare -A SVC_CIDR=(
    [cluster1]="10.1.0.0/16"
    [cluster2]="10.3.0.0/16"
)

# ── Prerequisites ─────────────────────────────────────────────────────────────

check_prerequisites() {
    local missing=()
    for cmd in kind kubectl git jq jinjanate; do
        if ! command -v "${cmd}" &>/dev/null; then
            missing+=("${cmd}")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        exit 1
    fi
}

# ── OVN-K fetch ───────────────────────────────────────────────────────────────

pull_ovn_image() {
    if docker image inspect "${OVN_IMAGE}" &>/dev/null; then
        log_info "OVN image already present locally — skipping pull."
        return 0
    fi
    log_info "Pulling OVN image: ${OVN_IMAGE}..."
    docker pull "${OVN_IMAGE}"
    log_info "OVN image pulled."
}

fetch_ovn_k() {
    if [[ -f "${OVN_K_DIR}/contrib/kind.sh" ]]; then
        log_info "OVN-K already present at ${OVN_K_DIR} — skipping clone."
        return 0
    fi
    log_info "Cloning OVN-K (depth=1, branch=${OVN_K_BRANCH}) into ${OVN_K_DIR}..."
    git clone --depth 1 --branch "${OVN_K_BRANCH}" "${OVN_K_REPO}" "${OVN_K_DIR}"
    log_info "OVN-K cloned."
}

# ── Cluster creation ──────────────────────────────────────────────────────────

create_cluster() {
    local name="$1"
    local pod_cidr="${POD_CIDR[${name}]}"
    local svc_cidr="${SVC_CIDR[${name}]}"

    # Do NOT check cluster_exists here — kind.sh handles it itself by deleting
    # and recreating any existing cluster.  This ensures a broken/partial cluster
    # is always fixed on re-run.
    log_info "Creating cluster '${name}' with OVN-K IC (image: ${OVN_IMAGE})..."

    # contrib/kind.sh creates the KIND cluster and deploys OVN-K in one step.
    # Flags used (mirrors Submariner's deploy_kind_ovn):
    #   -ov   use the specified image (no local build)
    #   -cn   cluster name
    #   -ic   enable interconnect mode
    #   -ric  run-in-container (required when kind.sh itself runs inside Docker)
    #   -wk   number of worker nodes
    #   -npz  nodes per zone (1 = each node is its own zone)
    #   --disable-ovnkube-identity  skip webhook cert setup (simpler for dev)
    NET_CIDR_IPV4="${pod_cidr}" \
    SVC_CIDR_IPV4="${svc_cidr}" \
    bash "${OVN_K_DIR}/contrib/kind.sh" \
        -ov  "${OVN_IMAGE}" \
        -cn  "${name}" \
        -ic \
        -ric \
        -wk  "${WORKERS}" \
        -npz 1 \
        --disable-ovnkube-identity

    log_info "Cluster '${name}' created with OVN-K IC."
}

# ── Main ──────────────────────────────────────────────────────────────────────

log_info "MCN KIND cluster setup — OVN-Kubernetes IC mode (via contrib/kind.sh)"
log_info "Clusters  : ${CLUSTERS}"
log_info "OVN-K     : ${OVN_K_REPO} @ ${OVN_K_BRANCH}"
log_info "OVN image : ${OVN_IMAGE}"

check_prerequisites
pull_ovn_image
fetch_ovn_k

for cluster in ${CLUSTERS}; do
    create_cluster "${cluster}"
done

# Export kubeconfigs and make them world-readable (written as root in container)
for cluster in ${CLUSTERS}; do
    kubeconfig="/tmp/mcn-${cluster}.kubeconfig"
    kind export kubeconfig --name "${cluster}" --kubeconfig "${kubeconfig}"
    chmod a+r "${kubeconfig}"
    print_cluster_summary "${cluster}" "${kubeconfig}"
done

log_info "All clusters are ready with OVN-K IC CNI."
log_info ""
log_info "To use the clusters:"
for cluster in ${CLUSTERS}; do
    log_info "  export KUBECONFIG=/tmp/mcn-${cluster}.kubeconfig   # ${cluster}"
done
log_info ""
log_info "Next step: make kind-load-only && make kind-deploy-all"
