# Deploy Runbook

## Prerequisites

- Terraform state bucket bootstrapped (`cd infra/bootstrap && terraform apply`)
- Demo YC cluster created (`cd infra && terraform apply -var-file=demo.tfvars`)
- YC CLI configured, `kubectl` available, `helm` >= 3.14

## Local render gate (no credentials needed)

```bash
helm lint deploy/chart
helm template yaxter deploy/chart -f deploy/chart/values-demo.yaml > /tmp/demo.yaml
helm template yaxter deploy/chart -f deploy/chart/values-prod.yaml > /tmp/prod.yaml
# optional: validate against k8s schema
# kubeconform -strict /tmp/demo.yaml /tmp/prod.yaml
```

## Bootstrap (one-time, [needs YC])

```bash
# 1. Create state bucket + YDB lock table
cd infra/bootstrap
terraform init
terraform apply -var="folder_id=$YC_FOLDER_ID" -var="cloud_id=$YC_CLOUD_ID"

# 2. Apply demo infra
cd ../
terraform init
terraform apply -var-file=demo.tfvars \
  -var="folder_id=$YC_FOLDER_ID" \
  -var="cloud_id=$YC_CLOUD_ID"

# 3. Capture outputs for Helm
terraform output -json > /tmp/tf-outputs.json
```

## Initial deploy ([needs YC])

```bash
# Get kubeconfig
yc managed-kubernetes cluster get-credentials <cluster-id> --external

# Install chart (first time)
helm upgrade --install yaxter deploy/chart \
  -f deploy/chart/values-demo.yaml \
  --set image.tag=<sha> \
  --set image.repository=cr.yandex/<registry-id>/yaxter \
  --set externalSecrets.lockboxSecretIds.postgresDsn=<lockbox-id> \
  --set externalSecrets.lockboxSecretIds.jwtSeed=<lockbox-id> \
  --wait --timeout=10m

# Verify
kubectl get pods,job
kubectl rollout status deployment/yaxter-api
```

## Smoke test ([needs YC])

```bash
ALB_DNS=$(terraform -chdir=infra output -raw alb_dns_name)

# Health check
curl -fsS "https://$ALB_DNS/healthz"

# Auth providers list (simplest live endpoint)
curl -fsS "https://$ALB_DNS/v1/auth/providers"
```

## Rolling-update zero-5xx check ([needs YC])

```bash
# Background load (light trickle)
while true; do curl -s -o /dev/null -w "%{http_code}\n" "https://$ALB_DNS/v1/auth/providers"; sleep 0.2; done &

# Trigger rolling update (new image SHA)
helm upgrade yaxter deploy/chart \
  -f deploy/chart/values-demo.yaml \
  --set image.tag=<new-sha> \
  --set image.repository=cr.yandex/<registry-id>/yaxter \
  --wait --timeout=10m

# Verify — zero 5xx during rollout (maxUnavailable: 0 enforced by chart)
```

## Teardown ([needs YC])

```bash
helm uninstall yaxter
cd infra && terraform destroy -var-file=demo.tfvars \
  -var="folder_id=$YC_FOLDER_ID" -var="cloud_id=$YC_CLOUD_ID"
# Bootstrap resources removed manually or via separate destroy
```

## CI/CD flow

On every PR touching `deploy/**` or `infra/**`:
- `helm-render`: lint + template with both value sets (no credentials needed)
- `terraform`: fmt-check + init + validate (no credentials needed)

On merge to `main` (when `YC_DEPLOY_ENABLED=true` repository variable is set):
1. `image` job builds and pushes `cr.yandex/<registry>/yaxter:<sha>`
2. `deploy` job runs `helm upgrade --install` against the demo cluster

Required repository secrets:
- `YC_SA_KEY_JSON` — service account key JSON (CI SA with deploy permissions)
- `YC_FOLDER_ID`, `YC_CLOUD_ID`, `YC_K8S_CLUSTER_ID`, `YC_CR_REGISTRY_ID`

Required repository variable:
- `YC_DEPLOY_ENABLED=true` — gate so PRs and forks without credentials don't attempt deploy
