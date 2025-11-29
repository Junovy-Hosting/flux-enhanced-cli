# Flux Enhanced CLI

A Go-based enhanced CLI for Flux that provides:

- Real-time Kubernetes event monitoring
- Colored output with emojis
- No job control issues (unlike shell scripts)
- Better error handling and status reporting
- Double Ctrl+C support (first cancels, second force exits)
- Periodic status updates during long reconciliations

## Building

```bash
# Simple build
go build -o flux-enhanced-cli .

# Build with version info
make build
```

Or install to your PATH:

```bash
make install
```

## Usage

```bash
# Reconcile a kustomization
./flux-enhanced-cli --kind kustomization --name my-app --namespace flux-system

# Reconcile a helmrelease
./flux-enhanced-cli --kind helmrelease --name my-app --namespace production

# Reconcile a git source
./flux-enhanced-cli --kind source --name my-repo --namespace flux-system

# Reconcile an OCI source
./flux-enhanced-cli --kind source --source-type oci --name my-oci-repo --namespace flux-system

# Don't wait for completion
./flux-enhanced-cli --kind kustomization --name my-app --wait=false

# Custom timeout (supports Go duration format: 5m, 1h, 30s)
./flux-enhanced-cli --kind kustomization --name my-app --timeout 10m

# Disable colored output
./flux-enhanced-cli --kind kustomization --name my-app --no-color

# Print version
./flux-enhanced-cli --version
```

## Options

| Flag            | Description                                        | Default       |
| --------------- | -------------------------------------------------- | ------------- |
| `--kind`        | Resource kind (kustomization, helmrelease, source) | _required_    |
| `--name`        | Resource name                                      | _required_    |
| `--namespace`   | Kubernetes namespace                               | `flux-system` |
| `--wait`        | Wait for reconciliation to complete                | `true`        |
| `--timeout`     | Timeout for waiting (Go duration format)           | `5m`          |
| `--source-type` | Source type when kind is 'source' (git, oci)       | `git`         |
| `--no-color`    | Disable colored output                             | `false`       |
| `--version`     | Print version information                          | `false`       |

## Environment Variables

| Variable     | Description                                            |
| ------------ | ------------------------------------------------------ |
| `KUBECONFIG` | Path to kubeconfig file (defaults to `~/.kube/config`) |
| `NO_COLOR`   | Disable colors when set (any value)                    |

## Interrupt Handling

- **First Ctrl+C**: Gracefully cancels the current operation
- **Second Ctrl+C** (within 2 seconds): Force exits immediately

This allows you to cancel a single reconciliation without exiting the entire process when used in scripts.

## Features

### Real-time Event Monitoring

Shows Kubernetes events as they happen during reconciliation:

```
│ ℹ️  [ReconciliationSucceeded] Reconciliation finished in 321.037679ms
│ ⚠️  [HealthCheckFailed] health check failed: deployment not ready
```

### Periodic Status Updates

When waiting for reconciliation, shows progress every 10 seconds:

```
│ ℹ️  Still waiting... (elapsed: 30s, remaining: 4m30s)
│ ℹ️  Current status: Ready=False (Dependencies do not meet ready condition)
```

### Warning Formatting

Kubernetes client warnings are formatted nicely:

```
│ ⚠️  v2beta1 HelmRelease is deprecated, upgrade to v2
```

### HelmRelease API Version

Automatically uses HelmRelease v2 API when available, falling back to v2beta1 for older clusters.

## License

MIT
