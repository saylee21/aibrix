kube-apiserver (envtest) at https://127.0.0.1:55002/
### T0: all three gateways started, one model pod exists (14:27:29)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| C | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
Cut the API server connection of A and C at 14:27:29
Deleted pod llama-pod-1 at 14:27:31; API server now says: pods "llama-pod-1" not found
C: /v1/models first returned 503 at 14:27:49 (20.0 s after the cut)
### T1: 30 s after the delete, A and C still cut off (14:28:01)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 503 | [llama-pod-1] |
### T2: 90 s after the delete, A and C still cut off (14:29:11)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [llama-8b] | [llama-pod-1] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 503 | [llama-pod-1] |
Restored the connection of A and C at 14:29:21
### T3: 45 s after restoring the connection (14:30:06)
| gateway | probe service "" | service gateway-plugin | /v1/models | pods in cache (from /metrics) |
|---|---|---|---|---|
| A | SERVING | SERVING | 200 [] | [] |
| B | SERVING | SERVING | 200 [] | [] |
| C | SERVING | SERVING | 200 [] | [] |
exit 0
