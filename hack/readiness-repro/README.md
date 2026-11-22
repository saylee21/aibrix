# Gateway readiness reproduction

Run on 2026-11-22: Apple M1, macOS, Go 1.27.2.

## Setup

- AIBrix gateway built unmodified from `origin/main` at `3aebef9c` (`go build -o gateway ./cmd/plugins`).
- A real kube-apiserver and etcd from envtest (Kubernetes 1.31.0), with the AIBrix CRDs from `config/crd/*/` installed.
- Redis on port 6390.
- One model pod, `llama-pod-1`, with label `model.aibrix.ai/name=llama-8b`. Its status is set to Running and Ready, with IP 10.0.0.7.
- Three gateways with default flags:
  - **A**: reaches the API server through a TCP proxy that the harness cuts. The cut closes live connections and drops new ones.
  - **B**: direct connection. This is the control.
  - **C**: like A, plus `--model-list-discovery-max-age 10s`.

## Steps

1. Start all three gateways.
2. Cut A and C.
3. Delete the pod.
4. Sample at +30 s and +90 s.
5. Restore the connection, then sample at +45 s.

Each sample reads three things from every gateway:
- gRPC health for service `""` (what the shipped probe checks) and for `gateway-plugin`
- `/v1/models`
- the pods in the gateway's cache, from the `gateway_pod_outstanding_requests` series on `/metrics`

In run 3, C's `/v1/models` is also polled every second to time detection. Run 4 also measures the RFC's proposed reachability check: one `LIST pods` with `limit=1`, using A's and B's kubeconfigs, before the cut, during it, and after.

## Results

| | Run 2 (`-v 2`) | Run 3 (`-v 6`) |
|---|---|---|
| Results file | `result-run2.md` | `result-run3.md` |

Run 4 (`-v 2`, plus the reachability check) is in `result-run4.md`.

Both runs give the same table: A stays SERVING and keeps the deleted pod, B drops it, and C returns 503 from `/v1/models` but stays SERVING and keeps the pod.

In runs 3 and 4, C's first 503 came 20.0 s after the cut.

In run 4, the reachability check:
- succeeded through both A and B before the cut
- failed through A during the cut (EOF after 10.0 s, despite a 5 s `rest.Config.Timeout`, because client-go retries) while succeeding through B in 3 ms
- succeeded through both after the reconnect

## Why no error is logged

From the run 3 log of gateway A (`-v 6`, available on request):

1. At the cut, all three watch streams end with `Unexpected EOF`.
2. client-go then retries each watch reconnect in rounds of up to 10 attempts, logging `with_retry.go: Got a Retry-After 1s response`.
3. In client-go v0.31.8, `tools/cache/reflector.go:445` keeps retriable watch errors (connection refused, or 429) inside `ListAndWatch`, logging only at `-v 4`.
4. So `Run` never calls the watch error handler (`reflector.go:305-308`). C's `Invalidate()` hook fired 0 times.
5. At `-v 2`, A logged no warning or error between the cut and the reconnect.

## How to rerun

From the repository root on this branch:

```
mkdir -p /tmp/readiness-run
go build -o /tmp/gateway ./cmd/plugins
KUBEBUILDER_ASSETS=$(setup-envtest use 1.31.x -p path) GW_VERBOSITY=2 \
  go run ./hack/readiness-repro /tmp/readiness-run
```

The harness looks for the gateway binary at `<outdir>/../gateway`, so the `/tmp` paths above line up. It needs `redis-server` on your PATH and uses ports 6390, 8091 to 8093, and 50061 to 50063.

The full gateway logs from runs 2 and 3 are not committed; they are available on request. Local paths in the results are replaced with `<workdir>`.
