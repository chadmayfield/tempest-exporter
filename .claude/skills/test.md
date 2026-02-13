---
name: test
description: Run all tests with race detection
user_invocable: true
---

Run the test suite:

```bash
go test ./... -race -count=1
```

If tests fail, analyze the output and fix the issues.
