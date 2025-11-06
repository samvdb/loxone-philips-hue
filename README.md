# Philips Hue Loxone Bridge


## Build & Run (Go)


```bash

```


## Retrieve API key

```

curl -k -X POST https://10.0.0.157/api -H "Content-Type: application/json" \
  -d '{
    "devicetype": "loxone_hue#one",
    "generateclientkey": true
  }'
```