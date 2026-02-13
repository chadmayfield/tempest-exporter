---
name: deploy
description: Build, push, and deploy to Kubernetes
user_invocable: true
---

Build, push, and deploy to Kubernetes:

```bash
ko apply -f deploy/
```

Requires `KO_DOCKER_REPO` to be set. Remind the user if it's not.

Verify the rollout:

```bash
kubectl rollout status deployment/tempest-exporter -n monitoring
```

Before first deploy, ensure the secret and configmap exist:
- Secret: `kubectl create secret generic tempest-exporter-token --namespace monitoring --from-literal=TEMPEST_TOKEN=...`
- ConfigMap: edit `deploy/configmap.yaml` with device/station IDs, then `kubectl apply -f deploy/configmap.yaml`
