# Skynet agent — dev manifests (operator-parity)

These YAMLs mirror what **skynet-operator** would reconcile: namespace, RBAC, and the **skynet-agent** `Deployment` wired through a `Secret` (broker API URL + token) and a `ConfigMap` (cluster ID, broker namespace, `SKYNET_ALLOCATED_*`, `SKYNET_POOL_*`).

## Apply by hand (after `make clusters` + broker tokens)

1. Build/load the image: `make build-agent`
2. For each member cluster, set kubeconfig and run `test/deploy-agents.sh`, **or**:

```bash
export KUBECONFIG=test/kubeconfig-cluster1.yaml
kubectl apply -f test/manifests/skynet-agent/namespace.yaml
kubectl apply -f test/manifests/skynet-agent/rbac.yaml
kubectl apply -f test/manifests/skynet-agent/deployment.yaml
# Then create Secret + ConfigMap (see test/deploy-agents.sh — uses kubectl create ... | apply)
```

`test/deploy-agents.sh` applies CRDs, static manifests, then creates/updates **`skynet-agent-broker`** (Secret) and **`skynet-agent-config`** (ConfigMap) per cluster.

## Broker prerequisites

- Namespace `skynet-broker`, SkyNet CRDs, per-cluster SA tokens (`test/setup-clusters-v2.sh`).
- ConfigMap **`skynet-broker-pools`** on the broker (same script).

## Verify BGP mesh

```bash
make verify-bgp
```
