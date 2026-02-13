---
name: lint
description: Run linting and static analysis
user_invocable: true
---

Run linting:

```bash
go vet ./...
```

If golangci-lint is installed, also run:

```bash
golangci-lint run
```

Fix any issues found.
