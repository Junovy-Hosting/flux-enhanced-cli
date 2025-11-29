package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/junovy-hosting/flux-enhanced-cli/pkg/events"
	"github.com/junovy-hosting/flux-enhanced-cli/pkg/output"
)

// Version information (set at build time with -ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// Kubernetes client warning pattern: W1123 13:40:53.387945   52532 warnings.go:70] message
var kubernetesWarningRegex = regexp.MustCompile(`^W\d+\s+\d+:\d+:\d+\.\d+\s+\d+\s+\S+:\d+\]\s+(.+)$`)

func processStderr(reader io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a Kubernetes client warning
		if matches := kubernetesWarningRegex.FindStringSubmatch(line); matches != nil {
			// Format the warning nicely
			output.PrintWarning(matches[1])
		} else if strings.TrimSpace(line) != "" {
			// Pass through other stderr output as-is
			fmt.Fprintf(os.Stderr, "%s\n", line)
		}
	}
}

func main() {
	var (
		kind       = flag.String("kind", "", "Resource kind (kustomization, helmrelease, source)")
		name       = flag.String("name", "", "Resource name")
		namespace  = flag.String("namespace", "flux-system", "Namespace")
		wait       = flag.Bool("wait", true, "Wait for reconciliation to complete")
		timeout    = flag.Duration("timeout", 5*time.Minute, "Timeout for waiting (e.g., 5m, 1h)")
		version    = flag.Bool("version", false, "Print version information and exit")
		noColor    = flag.Bool("no-color", false, "Disable colored output")
		sourceType = flag.String("source-type", "git", "Source type for 'source' kind (git, oci)")
	)
	flag.Parse()

	// Handle --version flag
	if *version {
		fmt.Printf("flux-enhanced-cli %s (built %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// Configure color output
	if *noColor || os.Getenv("NO_COLOR") != "" {
		output.DisableColors()
	}

	if *kind == "" || *name == "" {
		fmt.Fprintf(os.Stderr, "Error: --kind and --name are required\n")
		fmt.Fprintf(os.Stderr, "\nUsage: flux-enhanced-cli --kind <kind> --name <name> [options]\n")
		fmt.Fprintf(os.Stderr, "\nKinds: kustomization, helmrelease, source\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Handle signals with double Ctrl+C support (thread-safe)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var lastInterruptNano atomic.Int64
	var interruptCount atomic.Int32
	const interruptWindowNano = int64(2 * time.Second)

	go func() {
		for {
			<-sigChan
			nowNano := time.Now().UnixNano()
			lastNano := lastInterruptNano.Load()

			// Check if this is within the window of the last interrupt
			if nowNano-lastNano < interruptWindowNano {
				interruptCount.Add(1)
			} else {
				interruptCount.Store(1)
			}

			lastInterruptNano.Store(nowNano)
			count := interruptCount.Load()

			if count == 1 {
				// First interrupt: cancel gracefully
				fmt.Fprintf(os.Stderr, "\n⚠️  Interrupt received. Cancelling... (Press Ctrl+C again within 2s to force exit)\n")
				cancel()
			} else if count >= 2 {
				// Second interrupt: force exit
				fmt.Fprintf(os.Stderr, "\n⚠️  Force exit requested. Exiting immediately.\n")
				os.Exit(130) // Standard exit code for SIGINT
			}
		}
	}()

	// Validate source type
	validSourceTypes := map[string]bool{"git": true, "oci": true}
	if *kind == "source" && !validSourceTypes[*sourceType] {
		fmt.Fprintf(os.Stderr, "Error: invalid source-type '%s'. Valid types: git, oci\n", *sourceType)
		os.Exit(1)
	}

	// Start event monitoring (only if we have a valid kind for monitoring)
	var eventMonitor *events.Monitor
	if *kind == "kustomization" || *kind == "helmrelease" || *kind == "source" {
		var err error
		monitorKind := *kind
		if *kind == "source" {
			monitorKind = *sourceType // Pass "git" or "oci" to monitor
		}
		eventMonitor, err = events.NewMonitor(ctx, monitorKind, *name, *namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not start event monitoring: %v\n", err)
		} else {
			defer eventMonitor.Stop()
			go eventMonitor.Watch()
		}
	}

	// Build flux command
	var cmd *exec.Cmd
	if *kind == "source" {
		// For source, we need "flux reconcile source <type> <name>"
		cmd = exec.CommandContext(ctx, "flux", "reconcile", "source", *sourceType, *name, "-n", *namespace)
	} else {
		cmd = exec.CommandContext(ctx, "flux", "reconcile", *kind, *name, "-n", *namespace)
		if *kind == "kustomization" || *kind == "helmrelease" {
			cmd.Args = append(cmd.Args, "--with-source")
		}
	}

	// Run command and stream output
	output.PrintCommand(cmd.Args...)
	cmd.Stdout = os.Stdout

	// Intercept stderr to format warnings nicely
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stderr pipe: %v\n", err)
		os.Exit(1)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting flux: %v\n", err)
		os.Exit(1)
	}

	// Process stderr in a goroutine with WaitGroup to ensure completion
	var stderrWg sync.WaitGroup
	stderrWg.Add(1)
	go processStderr(stderrPipe, &stderrWg)

	// Wait for command to complete
	cmdErr := cmd.Wait()

	// Wait for stderr processing to complete before handling errors
	stderrWg.Wait()

	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running flux: %v\n", cmdErr)
		os.Exit(1)
	}

	// Wait for reconciliation if requested
	if *wait && eventMonitor != nil {
		output.PrintWaiting(*kind, *name)
		if err := eventMonitor.WaitForReady(ctx, *timeout); err != nil {
			output.PrintError(fmt.Sprintf("Reconciliation failed or timed out: %v", err))
			os.Exit(1)
		}
		output.PrintSuccess(*kind, *name)
	}
}
