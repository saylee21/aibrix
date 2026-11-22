#!/bin/bash
# envoy-test.sh <label> <n>: send n requests through Envoy, report status codes and
# how many request_start lines each gateway pod logged during the batch.
export PATH=$PATH:/usr/local/bin
label=$1; n=$2; ns=aibrix-system
pods=$(kubectl -n $ns get pods -l app=gateway-plugins -o jsonpath='{range .items[*]}{.metadata.name}{" "}{end}')
tmp=$(mktemp -d)
for p in $pods; do kubectl -n $ns logs $p 2>/dev/null | grep -c '"request_start"' > $tmp/$p; done
codes=$(for i in $(seq 1 $n); do curl -s -m 10 -o /dev/null -w "%{http_code}\n" http://localhost:8888/v1/chat/completions \
  -H 'Content-Type: application/json' -H 'Authorization: Bearer test-key-1234567890' \
  -d '{"model":"llama2-7b","messages":[{"role":"user","content":"hi"}],"max_tokens":5}'; done | sort | uniq -c | awk '{printf "%s x%s  ", $2, $1}')
echo "### $label ($(date +%T))"
echo "status codes: $codes"
kubectl -n $ns get endpointslice -l kubernetes.io/service-name=aibrix-gateway-plugins \
  -o jsonpath='{range .items[*].endpoints[*]}{.targetRef.name}{" ready="}{.conditions.ready}{"\n"}{end}' | sed 's/^/endpoint: /'
for p in $pods; do now=$(kubectl -n $ns logs $p 2>/dev/null | grep -c '"request_start"'); echo "requests handled by $p: $((now - $(cat $tmp/$p)))"; done
echo
