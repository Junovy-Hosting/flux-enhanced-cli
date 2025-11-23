# Flux Reconcile Enhanced

A Go-based wrapper around `flux reconcile` that provides:

- Real-time Kubernetes event monitoring
- Colored output with emojis
- No job control issues (unlike shell scripts)
- Better error handling and status reporting

## Building

```bash
go build -o flux-reconcile .
```

Or install to your PATH:

```bash
go install .
```

## Usage

```bash
# Reconcile a kustomization
./flux-reconcile --kind kustomization --name my-app --namespace flux-system

# Reconcile a helmrelease
./flux-reconcile --kind helmrelease --name my-app --namespace production

# Reconcile a git source
./flux-reconcile --kind source --name my-repo --namespace flux-system

# Don't wait for completion
./flux-reconcile --kind kustomization --name my-app --wait=false

# Custom timeout
./flux-reconcile --kind kustomization --name my-app --timeout 10m
```
