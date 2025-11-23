# Integration

The `flux-reconcile` binary can be integrated into shell scripts or other tooling to replace shell-based event monitoring.

## How It Works

The Go binary provides:

- Real-time Kubernetes event monitoring (no job control issues!)
- Colored output with emojis
- Automatic waiting for reconciliation completion
- Better error handling

## Building and Installation

```bash
go build -o flux-reconcile .
```

Or install to your PATH:

```bash
go install .
```

The binary can be used directly or called from shell scripts. Shell scripts can check for the binary and use it if available, falling back to direct `flux` commands if not found.

## Benefits

- ✅ No job control messages
- ✅ Real-time event monitoring during reconciliation
- ✅ Shows HealthCheckFailed and DependencyNotReady events
- ✅ Cleaner, more reliable output
- ✅ Better performance (Go vs shell loops)

## Usage in Scripts

Shell scripts can integrate this binary by:

1. Checking if the binary exists in a known location or PATH
2. If found, calling it with appropriate flags
3. If not found, falling back to direct `flux` commands
