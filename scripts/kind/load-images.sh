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

# Loads MCN images into one or more KIND clusters.
# Images must be built locally before running this script (make docker-build).
#
# Usage:
#   ./scripts/kind/load-images.sh
#   CLUSTERS="cluster1" ./scripts/kind/load-images.sh
#   VERSION=v0.1.0 ./scripts/kind/load-images.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=scripts/lib/utils.sh
source "${SCRIPT_DIR}/../lib/utils.sh"

REPO="${REPO:-quay.io/aswinsuryan/skynet}"
VERSION="${VERSION:-latest}"
CLUSTERS="${CLUSTERS:-cluster1 cluster2}"

OPERATOR_IMAGE="${REPO}:mcn-operator-${VERSION}"
BROKER_IMAGE="${REPO}:mcn-broker-${VERSION}"
AGENT_IMAGE="${REPO}:mcn-agent-${VERSION}"

IMAGES=(
    "${OPERATOR_IMAGE}"
    "${BROKER_IMAGE}"
    "${AGENT_IMAGE}"
)

load_images_into_cluster() {
    local cluster="$1"

    if ! cluster_exists "${cluster}"; then
        log_error "Cluster '${cluster}' does not exist. Run 'make kind-create' first."
        return 1
    fi

    log_info "Loading images into cluster '${cluster}'..."
    for image in "${IMAGES[@]}"; do
        log_info "  Loading ${image}..."
        kind load docker-image "${image}" --name "${cluster}"
    done
    log_info "All images loaded into '${cluster}'."
}

### Main ###

log_info "MCN image loader"
log_info "  Repo:     ${REPO}"
log_info "  Version:  ${VERSION}"
log_info "  Clusters: ${CLUSTERS}"
log_info "  Images:"
for image in "${IMAGES[@]}"; do
    log_info "    ${image}"
done

for cluster in ${CLUSTERS}; do
    load_images_into_cluster "${cluster}"
done

log_info "Done. Images are available in all specified clusters."
log_info "Next step: deploy the operator with 'make kind-deploy'"
