# Lana - OAuth SSO Authentication Server

Production-ready OAuth SSO authentication server written in Go. Provides OAuth 2.0 authentication through multiple providers (Google, Facebook) and issues JWTs for authenticated users.

## TL;DR

```bash
helm repo add lana https://iamolegga.github.io/lana
helm install my-lana lana/lana \
  --set-file config=my-config.yaml \
  --set secrets.COOKIE_SECRET="my-secret-32-chars-here-123456789"
```

## Introduction

This chart bootstraps a [Lana](https://github.com/iamolegga/lana) deployment on a [Kubernetes](https://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- OAuth credentials from providers (Google, Facebook, etc.)
- PersistentVolume or other storage for JWT private keys and login page files

## Installing the Chart

To install the chart with the release name `my-lana`:

```bash
helm install my-lana lana/lana
```

The command deploys Lana on the Kubernetes cluster in the default configuration.

> **Tip**: List all releases using `helm list`

## Uninstalling the Chart

To uninstall/delete the `my-lana` deployment:

```bash
helm delete my-lana
```

## Configuration

### Configuration Philosophy

This chart follows a simple philosophy:
- **`config`** = Lana's `config.yaml` structure (as YAML)
- **`secrets`** = Environment variables (free-form key-value pairs)
- **Certs** = User-provided volumes (PVC, Secret, etc.)
- **Login pages** = Either user-provided volumes, or uploaded at runtime via the optional admin endpoint (`config.admin.enabled=true`, see "Uploading Login Assets via the Admin Endpoint" below)

### The Four Configuration Patterns

#### 1. Development (Inline Everything)

```yaml
# values.yaml
config:
  cookie:
    secret: $COOKIE_SECRET
  ratelimit:
    requests_per_minute: 100
  hosts:
    localhost:8080:
      login_dir: /mnt/lana/login/
      jwt:
        private_key_file: /mnt/lana/certs/private.pem
        kid: "dev-key"
        audience: "http://localhost:8080"
        expiry: "1h"
      providers:
        google:
          client_id: $GOOGLE_CLIENT_ID
          client_secret: $GOOGLE_CLIENT_SECRET

secrets:
  COOKIE_SECRET: "my-dev-secret-32-chars-minimum-1234"
  GOOGLE_CLIENT_ID: "xxx.apps.googleusercontent.com"
  GOOGLE_CLIENT_SECRET: "GOCSPX-xxx"

extraVolumes:
  - name: lana-files
    hostPath:
      path: /path/to/local/lana/files
      type: Directory

extraVolumeMounts:
  - name: lana-files
    mountPath: /mnt/lana
    readOnly: true
```

```bash
helm install my-lana lana/lana -f values.yaml
```

#### 2. Production (Inline Config + Existing Secret)

Most common pattern for production deployments:

```yaml
# values.yaml
config:
  cookie:
    secret: $COOKIE_SECRET
  ratelimit:
    requests_per_minute: 100
    x_forwarded_for_index: -1
  logging:
    level: info
    format: json
  hosts:
    auth.example.com:
      login_dir: /mnt/lana/login/
      allowed_redirect_urls:
        - "https://*.example.com/*"
        - "https://example.com/*"
      jwt:
        private_key_file: /mnt/lana/certs/private.pem
        kid: "auth-example-prod"
        audience: "https://auth.example.com"
        expiry: "24h"
      providers:
        google:
          client_id: $GOOGLE_CLIENT_ID
          client_secret: $GOOGLE_CLIENT_SECRET

existingSecret:
  name: "lana-prod-secrets"  # Created externally (e.g., via External Secrets Operator)

extraVolumes:
  - name: lana-files
    persistentVolumeClaim:
      claimName: lana-pvc

extraVolumeMounts:
  - name: lana-files
    mountPath: /mnt/lana
    readOnly: true

ingress:
  enabled: true
  ingressClassName: nginx
  hostname: auth.example.com
  tls: true
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod

resources:
  limits:
    cpu: 200m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPU: 70
```

```bash
# Create the secret externally
kubectl create secret generic lana-prod-secrets \
  --from-literal=COOKIE_SECRET="your-32-char-secret-here-12345678901" \
  --from-literal=GOOGLE_CLIENT_ID="xxx.apps.googleusercontent.com" \
  --from-literal=GOOGLE_CLIENT_SECRET="GOCSPX-xxx"

# Install the chart
helm install my-lana lana/lana -f values.yaml
```

#### 3. GitOps (Existing Config + Existing Secret)

Full GitOps approach with externally managed resources:

```yaml
# values.yaml
existingConfig:
  name: "lana-config"

existingSecret:
  name: "lana-secrets"

extraVolumes:
  - name: lana-files
    persistentVolumeClaim:
      claimName: lana-pvc

extraVolumeMounts:
  - name: lana-files
    mountPath: /mnt/lana
    readOnly: true

ingress:
  enabled: true
  ingressClassName: nginx
  hostname: auth.example.com
  tls: true
```

```bash
# Create ConfigMap
kubectl create configmap lana-config --from-file=config.yaml

# Create Secret (e.g., via External Secrets Operator or Sealed Secrets)
kubectl create secret generic lana-secrets \
  --from-literal=COOKIE_SECRET="xxx" \
  --from-literal=GOOGLE_CLIENT_ID="xxx" \
  --from-literal=GOOGLE_CLIENT_SECRET="xxx"

# Install the chart
helm install my-lana lana/lana -f values.yaml
```

#### 4. Multi-Host Setup

Single deployment serving multiple OAuth hosts:

```yaml
# values.yaml
config:
  cookie:
    secret: $COOKIE_SECRET
  ratelimit:
    requests_per_minute: 100
  logging:
    level: info
    format: json
  hosts:
    auth.prod.example.com:
      login_dir: /mnt/lana/login/prod
      allowed_redirect_urls:
        - "https://*.prod.example.com/*"
      jwt:
        private_key_file: /mnt/lana/certs/prod-private.pem
        kid: "prod-key"
        audience: "https://auth.prod.example.com"
        expiry: "24h"
      providers:
        google:
          client_id: $GOOGLE_CLIENT_ID_PROD
          client_secret: $GOOGLE_CLIENT_SECRET_PROD

    auth.staging.example.com:
      login_dir: /mnt/lana/login/staging
      allowed_redirect_urls:
        - "https://*.staging.example.com/*"
      jwt:
        private_key_file: /mnt/lana/certs/staging-private.pem
        kid: "staging-key"
        audience: "https://auth.staging.example.com"
        expiry: "1h"
      providers:
        google:
          client_id: $GOOGLE_CLIENT_ID_STAGING
          client_secret: $GOOGLE_CLIENT_SECRET_STAGING
        facebook:
          client_id: $FACEBOOK_CLIENT_ID_STAGING
          client_secret: $FACEBOOK_CLIENT_SECRET_STAGING

secrets:
  COOKIE_SECRET: "shared-secret-32-chars-minimum-123"
  GOOGLE_CLIENT_ID_PROD: "xxx-prod.apps.googleusercontent.com"
  GOOGLE_CLIENT_SECRET_PROD: "GOCSPX-prod-xxx"
  GOOGLE_CLIENT_ID_STAGING: "xxx-staging.apps.googleusercontent.com"
  GOOGLE_CLIENT_SECRET_STAGING: "GOCSPX-staging-xxx"
  FACEBOOK_CLIENT_ID_STAGING: "123456789"
  FACEBOOK_CLIENT_SECRET_STAGING: "xxx"
```

### Uploading Login Assets via the Admin Endpoint

For per-host login HTML/CSS/JS too large to fit in a ConfigMap (1 MiB limit) and too tedious to bake into the image on every change, the chart can provision a writable PVC and a separate admin HTTP listener that accepts ZIP uploads. The admin listener is exposed as a **ClusterIP Service with no Ingress** — it's only reachable via `kubectl port-forward`, an in-cluster Job, or a cluster-internal CI runner.

Enable it in values:

```yaml
config:
  cookie:
    secret: $COOKIE_SECRET
  admin:
    enabled: true
    port: 8081
  hosts:
    auth.example.com:
      # NOTE: do not set login_dir here. The chart computes it from the host name
      # (auth.example.com -> /var/lana/login/auth-example-com/) and will fail
      # `helm install/upgrade` if you set login_dir explicitly.
      allowed_redirect_urls:
        - "https://*.example.com/*"
      jwt:
        private_key_file: /mnt/lana/certs/private.pem
        kid: "auth-example-prod"
        audience: "https://auth.example.com"
        expiry: "24h"
      providers:
        google:
          client_id: $GOOGLE_CLIENT_ID
          client_secret: $GOOGLE_CLIENT_SECRET

loginAssets:
  mountPath: /var/lana/login
  storageClass: ""        # use cluster default
  size: 1Gi
  accessMode: ReadWriteOnce  # switch to ReadWriteMany if replicaCount > 1
```

When enabled, the chart renders:

- A **PVC** (`<release>-lana-login-assets`) mounted at `loginAssets.mountPath`.
- A second **Service** (`<release>-lana-admin`) exposing `config.admin.port` — no Ingress is created; expose it yourself only if you need external access.
- A **busybox init container** that `mkdir -p`s each host's login directory on the PVC before the main container starts.
- An additional **`containerPort`** on the main container for the admin listener.
- **`readOnlyRootFilesystem: true`** on the main container (the PVC is the only writable mount).

The chart computes `login_dir` for each host using the rule `lower | replace "." "-" | replace ":" "-"`:

| Host name | Resulting `login_dir` |
|-----------|-----------------------|
| `auth.example.com` | `/var/lana/login/auth-example-com/` |
| `auth.example.com:8443` | `/var/lana/login/auth-example-com-8443/` |

#### Uploading a host's assets

There is no authentication on the admin endpoint — access control relies on Kubernetes RBAC (whoever has `port-forward`/`exec` on the namespace can upload, which is the same set of people who can overwrite files on the PVC directly). The threat model assumes a trusted single-tenant cluster.

```bash
# Port-forward from the admin Service
kubectl port-forward svc/my-lana-admin 8081:8081 &

# Pack the login directory into a ZIP
(cd ./login && zip -r /tmp/login.zip .)

# Upload — the {host} path segment must match a configured host name
curl -X POST \
  -H "Content-Type: application/zip" \
  --data-binary @/tmp/login.zip \
  http://localhost:8081/admin/login-assets/auth.example.com
# => {"status":"ok"}
```

The new content is served from `/` on the public port immediately — no restart required.

**Safety:**
- ZIP entries with `..`, absolute paths, or symlink mode bits are rejected (`400 Bad Request`).
- Uploads for hosts not in `config.hosts` are rejected (`404 Not Found`).
- Extraction uses a sibling temp directory on the PVC, then `os.RemoveAll` + `os.Rename` for an atomic swap.

Upload activity is captured by the existing Prometheus middleware, so `http_requests_total` and `http_request_duration_seconds` labeled with `path=/admin/login-assets/<host>` show up on the public `/metrics` endpoint alongside OAuth traffic.

### Providing Files (Certs and Login Pages)

Lana requires file-based resources. Provide them via `extraVolumes`:

#### Option 1: PersistentVolumeClaim

```yaml
extraVolumes:
  - name: lana-files
    persistentVolumeClaim:
      claimName: lana-pvc

extraVolumeMounts:
  - name: lana-files
    mountPath: /mnt/lana
    readOnly: true
```

Expected structure:
```
/mnt/lana/
├── certs/
│   ├── private.pem          # JWT signing key
│   └── staging-private.pem  # Additional keys for multi-host
└── login/
    └── index.html          # Login page
```

#### Option 2: ConfigMap for Login Page

```yaml
extraVolumes:
  - name: certs
    secret:
      secretName: lana-jwt-keys
  - name: login-page
    configMap:
      name: lana-login-page

extraVolumeMounts:
  - name: certs
    mountPath: /mnt/lana/certs
    readOnly: true
  - name: login-page
    mountPath: /mnt/lana/login
    readOnly: true
```

#### Option 3: Secret for JWT Keys

```bash
# Create secret with JWT private key
kubectl create secret generic lana-jwt-keys \
  --from-file=private.pem=./private.pem
```

### Generating Secrets

#### Generate Cookie Secret

```bash
openssl rand -base64 32
```

#### Generate JWT RSA Private Key

```bash
openssl genrsa -out private.pem 2048
```

Extract public key for JWT verification:

```bash
openssl rsa -in private.pem -pubout -out public.pem
```

## Parameters

### Global Parameters

| Name                      | Description                                     | Value |
| ------------------------- | ----------------------------------------------- | ----- |
| `global.imageRegistry`    | Global Docker image registry                    | `""`  |
| `global.imagePullSecrets` | Global Docker registry secret names as an array | `[]`  |

### Common Parameters

| Name                | Description                                        | Value |
| ------------------- | -------------------------------------------------- | ----- |
| `nameOverride`      | String to partially override common.names.fullname | `""`  |
| `fullnameOverride`  | String to fully override common.names.fullname     | `""`  |
| `commonLabels`      | Labels to add to all deployed objects              | `{}`  |
| `commonAnnotations` | Annotations to add to all deployed objects         | `{}`  |

### Image Parameters

| Name                | Description                    | Value                |
| ------------------- | ------------------------------ | -------------------- |
| `image.registry`    | Lana image registry            | `ghcr.io`            |
| `image.repository`  | Lana image repository          | `iamolegga/lana`     |
| `image.tag`         | Lana image tag                 | `v0.1.0`             |
| `image.pullPolicy`  | Lana image pull policy         | `IfNotPresent`       |

### Lana Configuration Parameters

| Name                           | Description                                                      | Value |
| ------------------------------ | ---------------------------------------------------------------- | ----- |
| `config`                       | Lana configuration as structured YAML (matches config.yaml)      | `{}`  |
| `existingConfig.name`          | Name of existing ConfigMap containing config.yaml                | `""`  |
| `existingConfig.namespace`     | Namespace of existing ConfigMap (defaults to release namespace)  | `""`  |
| `secrets`                      | Environment variables for Lana (plain text, auto-base64 encoded) | `{}`  |
| `existingSecret.name`          | Name of existing Secret containing environment variables         | `""`  |
| `existingSecret.namespace`     | Namespace of existing Secret (defaults to release namespace)     | `""`  |

**Important**:
- `config` matches Lana's `config.yaml` structure exactly
- `secrets` is completely free-form - define ANY environment variables your config needs
- Both `config` and `secrets` support environment variable substitution (e.g., `$COOKIE_SECRET`)

### Deployment Parameters

| Name                                   | Description                                | Value           |
| -------------------------------------- | ------------------------------------------ | --------------- |
| `replicaCount`                         | Number of Lana replicas to deploy          | `1`             |
| `updateStrategy.type`                  | Lana deployment strategy type              | `RollingUpdate` |
| `podSecurityContext.enabled`           | Enabled Lana pods' Security Context        | `true`          |
| `podSecurityContext.fsGroup`           | Set Lana pod's Security Context fsGroup    | `1001`          |
| `containerSecurityContext.enabled`     | Enabled Lana containers' Security Context  | `true`          |
| `containerSecurityContext.runAsUser`   | Set Lana containers' Security Context runAsUser | `1001`    |
| `resources.limits`                     | The resources limits for the Lana containers | `{}`          |
| `resources.requests`                   | The requested resources for the Lana containers | `{}`       |
| `extraVolumes`                         | Extra volumes for the Lana pod(s)          | `[]`            |
| `extraVolumeMounts`                    | Extra volume mounts for the Lana container(s) | `[]`         |

### Service Parameters

| Name                  | Description              | Value       |
| --------------------- | ------------------------ | ----------- |
| `service.type`        | Lana service type        | `ClusterIP` |
| `service.ports.http`  | Lana service HTTP port   | `8080`      |

### Login Assets Parameters

Used only when `config.admin.enabled=true`. See "Uploading Login Assets via the Admin Endpoint" above.

| Name                       | Description                                                       | Value              |
| -------------------------- | ----------------------------------------------------------------- | ------------------ |
| `loginAssets.mountPath`    | Where the assets PVC is mounted inside the pod                    | `/var/lana/login`  |
| `loginAssets.storageClass` | StorageClass for the PVC (empty = cluster default)                | `""`               |
| `loginAssets.size`         | PVC size                                                          | `1Gi`              |
| `loginAssets.accessMode`   | PVC access mode (use `ReadWriteMany` for `replicaCount > 1`)      | `ReadWriteOnce`    |

### Ingress Parameters

| Name                       | Description                          | Value                |
| -------------------------- | ------------------------------------ | -------------------- |
| `ingress.enabled`          | Enable ingress record generation     | `false`              |
| `ingress.hostname`         | Default host for the ingress record  | `lana.local`         |
| `ingress.ingressClassName` | IngressClass name                    | `""`                 |
| `ingress.tls`              | Enable TLS configuration             | `false`              |
| `ingress.annotations`      | Additional annotations for Ingress   | `{}`                 |

### Autoscaling Parameters

| Name                       | Description                                | Value   |
| -------------------------- | ------------------------------------------ | ------- |
| `autoscaling.enabled`      | Enable Horizontal POD autoscaling          | `false` |
| `autoscaling.minReplicas`  | Minimum number of Lana replicas            | `1`     |
| `autoscaling.maxReplicas`  | Maximum number of Lana replicas            | `11`    |
| `autoscaling.targetCPU`    | Target CPU utilization percentage          | `50`    |

### Health Check Parameters

| Name                                  | Description                          | Value  |
| ------------------------------------- | ------------------------------------ | ------ |
| `livenessProbe.enabled`               | Enable livenessProbe                 | `true` |
| `livenessProbe.initialDelaySeconds`   | Initial delay seconds                | `10`   |
| `readinessProbe.enabled`              | Enable readinessProbe                | `true` |
| `readinessProbe.initialDelaySeconds`  | Initial delay seconds                | `5`    |

### Monitoring Parameters

| Name                            | Description                                    | Value   |
| ------------------------------- | ---------------------------------------------- | ------- |
| `serviceMonitor.enabled`        | Create ServiceMonitor for Prometheus Operator  | `false` |
| `serviceMonitor.interval`       | Interval at which metrics should be scraped    | `30s`   |

### Network Policy Parameters

| Name                         | Description                        | Value   |
| ---------------------------- | ---------------------------------- | ------- |
| `networkPolicy.enabled`      | Enable network policies            | `false` |
| `networkPolicy.allowExternal`| Allow external connections         | `true`  |

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -l app.kubernetes.io/name=lana
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

### Verify Configuration

```bash
# View the generated ConfigMap
kubectl get configmap <release-name>-lana -o yaml

# View the config content
kubectl get configmap <release-name>-lana -o jsonpath='{.data.config\.yaml}'

# Check environment variables in the pod
kubectl exec -it <pod-name> -- env | grep -E 'COOKIE|GOOGLE|FACEBOOK'
```

### Test Installation with Dry-Run

```bash
helm install my-lana lana/lana -f values.yaml --dry-run --debug
```

### Common Issues

**Issue: Pods not starting**
- Check that configuration is valid YAML
- Verify all environment variables referenced in config are provided in secrets
- Check that volumes for certs and login files are properly mounted

**Issue: OAuth login not working**
- Verify OAuth client ID and secret are correct
- Check that allowed redirect URLs match your application
- Ensure the OAuth provider callback URL is configured: `https://your-host/oauth/callback/{provider}`

**Issue: JWT verification failing**
- Confirm JWT private key is in correct PEM format
- Verify the `kid` (Key ID) is set correctly
- Check the `audience` claim matches your application
- Ensure the public key is extracted and shared with consuming services

**Issue: Configuration not updating**
- Check if using `existingConfig` - external ConfigMap changes require pod restart
- For inline `config`, Helm upgrade should trigger rolling update (checksum annotation changes)

## License

Copyright 2025 iamolegga

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
