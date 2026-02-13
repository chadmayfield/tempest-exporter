---
name: release
description: Create a new release
user_invocable: true
---

Create a new release:

1. Ask for the version number (suggest next semver based on git tags)
2. Run tests first — abort if they fail:
   ```bash
   go test ./... -race -count=1
   ```
3. Create an annotated tag:
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   ```
4. Push the tag:
   ```bash
   git push origin vX.Y.Z
   ```
5. Confirm the GitHub Action will build and push the tagged image
