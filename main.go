package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobbydrake/flux-reconcile/pkg/events"
	"github.com/bobbydrake/flux-reconcile/pkg/output"
)

func main() {
	var (
		kind      = flag.String("kind", "", "Resource kind (kustomization, helmrelease, source)")
		name      = flag.String("name", "", "Resource name")
		namespace = flag.String("namespace", "flux-system", "Namespace")
		wait      = flag.Bool("wait", true, "Wait for reconciliation to complete")
		timeout   = flag.Duration("timeout", 5*time.Minute, "Timeout for waiting")
	)
	flag.Parse()

	if *kind == "" || *name == "" {
		fmt.Fprintf(os.Stderr, "Error: --kind and --name are required\n")
		os.Exit(1)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Start event monitoring (only if we have a valid kind for monitoring)
	var eventMonitor *events.Monitor
	if *kind == "kustomization" || *kind == "helmrelease" || *kind == "source" {
		var err error
		eventMonitor, err = events.NewMonitor(ctx, *kind, *name, *namespace)
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
		// For source, we need "flux reconcile source git <name>"
		cmd = exec.CommandContext(ctx, "flux", "reconcile", "source", "git", *name, "-n", *namespace)
	} else {
		cmd = exec.CommandContext(ctx, "flux", "reconcile", *kind, *name, "-n", *namespace)
		if *kind == "kustomization" || *kind == "helmrelease" {
			cmd.Args = append(cmd.Args, "--with-source")
		}
	}

	// Run command and stream output
	output.PrintCommand(cmd.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running flux: %v\n", err)
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
