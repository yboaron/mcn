#!/bin/bash

# Check prerequisites for SkyNet test environment

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== SkyNet Test Environment Prerequisites Check ==="
echo ""

all_good=true

# Check Docker
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✓${NC} Docker installed"
else
    echo -e "${RED}✗${NC} Docker not found"
    echo "  Install: https://docs.docker.com/get-docker/"
    all_good=false
fi

# Check KIND
if command -v kind &> /dev/null; then
    echo -e "${GREEN}✓${NC} KIND installed ($(kind version | head -1))"
else
    echo -e "${RED}✗${NC} KIND not found"
    echo "  Install: curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64"
    echo "           chmod +x ./kind && sudo mv ./kind /usr/local/bin/"
    all_good=false
fi

# Check kubectl
if command -v kubectl &> /dev/null; then
    echo -e "${GREEN}✓${NC} kubectl installed ($(kubectl version --client -o json 2>/dev/null | grep gitVersion | head -1))"
else
    echo -e "${RED}✗${NC} kubectl not found"
    echo "  Install: https://kubernetes.io/docs/tasks/tools/"
    all_good=false
fi

# Check git
if command -v git &> /dev/null; then
    echo -e "${GREEN}✓${NC} git installed"
else
    echo -e "${RED}✗${NC} git not found"
    echo "  Install: sudo apt-get install git  (or yum install git)"
    all_good=false
fi

# Check Go (optional, for building)
if command -v go &> /dev/null; then
    echo -e "${GREEN}✓${NC} Go installed ($(go version))"
else
    echo -e "${YELLOW}!${NC} Go not found (optional, needed for building agent)"
    echo "  Install: https://go.dev/doc/install"
fi

echo ""
echo "--- OVN-Kubernetes Repository ---"

# Check OVNK_REPO_PATH
if [ -z "$OVNK_REPO_PATH" ]; then
    echo -e "${YELLOW}!${NC} OVNK_REPO_PATH not set"
    echo "  This environment variable must point to your OVN-Kubernetes repository"
    echo ""
    echo "  Options:"
    echo "    1. Use existing repo (if you have it):"
    echo "       export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes"
    echo ""
    echo "    2. Clone a new one:"
    echo "       git clone https://github.com/ovn-org/ovn-kubernetes ~/ovn-kubernetes"
    echo "       export OVNK_REPO_PATH=~/ovn-kubernetes"
    all_good=false
elif [ ! -d "$OVNK_REPO_PATH" ]; then
    echo -e "${RED}✗${NC} OVNK_REPO_PATH set but directory doesn't exist: $OVNK_REPO_PATH"
    all_good=false
elif [ ! -f "$OVNK_REPO_PATH/contrib/kind.sh" ]; then
    echo -e "${RED}✗${NC} OVNK_REPO_PATH set but contrib/kind.sh not found"
    echo "  Path: $OVNK_REPO_PATH"
    echo "  This doesn't appear to be a valid OVN-Kubernetes repository"
    all_good=false
else
    echo -e "${GREEN}✓${NC} OVNK_REPO_PATH set and valid: $OVNK_REPO_PATH"
fi

echo ""
echo "==========================================="

if [ "$all_good" = true ]; then
    echo -e "${GREEN}✓ All prerequisites met!${NC}"
    echo ""
    echo "Ready to run:"
    echo "  ./setup-clusters.sh"
    echo ""
    echo "Or for full automated setup:"
    echo "  ./run-all.sh"
    exit 0
else
    echo -e "${RED}✗ Some prerequisites missing${NC}"
    echo ""
    echo "Please install missing components and set OVNK_REPO_PATH"
    echo ""
    echo "Quick setup:"
    echo "  export OVNK_REPO_PATH=/home/yboaron/prj/ovn-kubernetes"
    echo "  ./setup-clusters.sh"
    exit 1
fi
