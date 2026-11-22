# Envoy behaviour with an unready gateway replica (kind)

Run on 2026-11-22: Apple M1, Docker Desktop 29.9.3 (8 CPUs, about 7.7 GB), kind 0.33.0, kindest/node v1.32.0 (one control plane, one worker; `hack/ci/kind-config.yaml`).

AIBrix images were built unmodified from `origin/main` at `3aebef9c`: `controller-manager`, `gateway-plugins`, and `vllm-mock` from `development/app`.

## Setup, following `test/run-e2e-tests.sh`

```
kind create cluster --name aibrix-readiness --image kindest/node:v1.32.0 --config hack/ci/kind-config.yaml
kubectl apply -k config/dependency --server-side   # Envoy Gateway, KubeRay CRDs
kubectl apply -k config/crd --server-side          # AIBrix CRDs (config/default does not include them)
kind load docker-image --name aibrix-readiness aibrix/controller-manager:nightly aibrix/gateway-plugins:nightly aibrix/vllm-mock:nightly
kubectl apply -k config/default --server-side
kubectl -n aibrix-system scale deploy aibrix-metadata-service --replicas=0    # image not built; not needed for routing
cd development/app && kubectl apply -k config/mock
kubectl scale deploy mock-qwen3-8b mock-qwen3-8b-replica-inflight mock-qwen3-8b-replica-rps --replicas=0
kubectl patch deploy mock-llama2-7b --type strategic \
  -p '{"spec":{"replicas":2,"template":{"spec":{"containers":[{"name":"aibrix-runtime","$patch":"delete"}]}}}}'
kubectl -n envoy-gateway-system port-forward svc/envoy-aibrix-system-aibrix-eg-903790dc 8888:80
```

## Two gateway replicas behind one Service

The Service `aibrix-gateway-plugins` selects `app=gateway-plugins`.

A second Deployment, `aibrix-gateway-plugins-b`, is a copy of the shipped one with three changes:
- an extra label, `variant: b`
- 1 replica
- a readiness probe of `grpc: {port: 50052, service: <name>}`

The Service selects both replicas. The readiness probe is then pointed at `service: simulated-stale`, which the health server does not know, so the probe fails. That is what a replica reporting NOT_SERVING on its readiness service looks like to Kubernetes.

`envoy-test.sh <label> <n>` sends n chat requests through Envoy and counts `request_start` log lines per gateway pod.

## Results (`envoy-results.md`)

| Step | Endpoints ready | Client status | Requests per gateway |
|---|---|---|---|
| 1. control, both ready | a, b | 200 x40 | a 21, b 19 |
| 2. b's readiness probe fails | a | 200 x40 | a 40, b 0 (b alive, 0 restarts) |
| 3. both readiness probes fail | none | 500 x10 | a 0, b 0 |
| 4. a restored | a | 200 x20 | a 20, b 0 |

`failOpen` is unset on `aibrix-gateway-plugins-extension-policy`, which Envoy Gateway treats as false.

## Side observation

Before the CRDs were installed, the gateway's informers could not sync, so port 50052 never opened. The liveness probe, which checks the same port as readiness, then restarted the gateway 5 times: `Liveness probe failed: timeout: failed to connect service "10.244.1.6:50052" within 1s`.

## What this does not show

In step 2, changing the probe rolled a new b pod that was never ready. A pod that was ready and then turns NOT_SERVING mid-life needs the PR 2 code. Envoy reads the same EndpointSlice ready condition in both cases, so the same result is expected, but it is not measured here.
