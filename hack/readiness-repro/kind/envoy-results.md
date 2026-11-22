### Step 1 (control): both gateways ready (15:17:00)
status codes: 200 x40  
endpoint: aibrix-gateway-plugins-859b476cb5-gzm9x ready=true
endpoint: aibrix-gateway-plugins-b-d5558bbc4-kk8v6 ready=true
requests handled by aibrix-gateway-plugins-859b476cb5-gzm9x: 21
requests handled by aibrix-gateway-plugins-b-d5558bbc4-kk8v6: 19

### Step 2: gateway b not ready (readiness probe fails), gateway a ready (15:19:20)
status codes: 200 x40  
endpoint: aibrix-gateway-plugins-859b476cb5-gzm9x ready=true
endpoint: aibrix-gateway-plugins-b-79658ff566-l5nwf ready=false
requests handled by aibrix-gateway-plugins-859b476cb5-gzm9x: 40
requests handled by aibrix-gateway-plugins-b-79658ff566-l5nwf: 0

### Step 3: both gateways not ready (failOpen false) (15:19:51)
status codes: 500 x10  
endpoint: aibrix-gateway-plugins-b-79658ff566-l5nwf ready=false
endpoint: aibrix-gateway-plugins-7f947cf6f-dtwnq ready=false
requests handled by aibrix-gateway-plugins-7f947cf6f-dtwnq: 0
requests handled by aibrix-gateway-plugins-b-79658ff566-l5nwf: 0

### Step 4: gateway a restored to ready (15:20:23)
status codes: 200 x20  
endpoint: aibrix-gateway-plugins-b-79658ff566-l5nwf ready=false
endpoint: aibrix-gateway-plugins-859b476cb5-hl6dz ready=true
requests handled by aibrix-gateway-plugins-859b476cb5-hl6dz: 20
requests handled by aibrix-gateway-plugins-b-79658ff566-l5nwf: 0

