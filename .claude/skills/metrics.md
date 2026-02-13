---
name: metrics
description: Run the exporter locally and check its output
user_invocable: true
---

Run the exporter locally and check its output:

1. Start the exporter: `go run .` (in background)
2. Wait 2 seconds for startup
3. `curl -s localhost:8080/metrics | grep tempest_`
4. Show the output
5. Stop the background process

Note: Requires TEMPEST_TOKEN, TEMPEST_DEVICE_ID, and TEMPEST_STATION_ID environment variables to be set.
