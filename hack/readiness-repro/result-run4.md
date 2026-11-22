kube-apiserver (envtest) at https://127.0.0.1:56039/
### T0: all three gateways started, one model pod exists (14:38:53)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| C | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
- cheap check via A kubeconfig: ok in 4ms, 1 item(s) returned
- cheap check via B kubeconfig: ok in 1ms, 1 item(s) returned
Cut the API server connection of A and C at 14:38:53
Deleted pod llama-pod-1 at 14:38:55; API server now says: pods "llama-pod-1" not found
C: /v1/models first returned 503 at 14:39:13 (20.0 s after the cut)
### T1: 30 s after the delete, A and C still cut off (14:39:25)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 503 | [llama-pod-1] |
- cheap check via A kubeconfig: FAILED after 10.027s (Get "https://127.0.0.1:56136/api/v1/pods?limit=1": EOF)
- cheap check via B kubeconfig: ok in 3ms, 0 item(s) returned
### T2: 90 s after the delete, A and C still cut off (14:40:45)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 503 | [llama-pod-1] |
Restored the connection of A and C at 14:40:55
### T3: 45 s after restoring the connection (14:41:40)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [] | [] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 200 [] | [] |
- cheap check via A kubeconfig: ok in 4ms, 0 item(s) returned
- cheap check via B kubeconfig: ok in 3ms, 0 item(s) returned
exit 0
