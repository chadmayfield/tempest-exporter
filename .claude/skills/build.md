---
name: build
description: Build the binary or container image
user_invocable: true
---

Build the Go binary:

```bash
go build -o tempest-exporter .
```

To build a container image with ko:

```bash
ko build . --local
```
