#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "Generating deepcopy code..."
controller-gen object:headerFile="${ROOT_DIR}/hack/boilerplate.go.txt" \
    paths="${ROOT_DIR}/pkg/apis/..."

echo "Generating CRD manifests..."
controller-gen crd:crdVersions=v1 \
    paths="${ROOT_DIR}/pkg/apis/..." \
    output:crd:artifacts:config="${ROOT_DIR}/deploy/crds"

echo "Code generation complete!"
