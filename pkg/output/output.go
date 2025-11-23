package output

import (
	"fmt"
	"os"
	"strings"
)

const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorBlue    = "\033[34m"
	ColorCyan    = "\033[36m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorRed     = "\033[31m"
	ColorMagenta = "\033[35m"
	ColorSubLog  = "\033[38;5;244m"
)

func isTerminal() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func PrintCommand(args ...string) {
	if !isTerminal() {
		fmt.Printf("│ %s\n", strings.Join(args, " "))
		return
	}
	fmt.Printf("%s│ %s%s\n", ColorSubLog, strings.Join(args, " "), ColorReset)
}

func PrintSublog(message string) {
	if !isTerminal() {
		fmt.Printf("│ %s\n", message)
		return
	}
	fmt.Printf("%s│ %s%s\n", ColorSubLog, message, ColorReset)
}

func PrintWaiting(kind, name string) {
	if !isTerminal() {
		fmt.Printf("⏳ Waiting for %s reconciliation...\n", kind)
		return
	}
	fmt.Printf("%s│ ⏳ Waiting for %s reconciliation...%s\n", ColorSubLog, kind, ColorReset)
}

func PrintSuccess(kind, name string) {
	if !isTerminal() {
		fmt.Printf("✅ %s reconciliation completed successfully\n", kind)
		return
	}
	fmt.Printf("%s│ ✅ %s reconciliation completed successfully%s\n", ColorSubLog, kind, ColorReset)
}

func PrintError(message string) {
	if !isTerminal() {
		fmt.Printf("❌ %s\n", message)
		return
	}
	fmt.Printf("%s│ %s❌ %s%s\n", ColorSubLog, ColorRed, message, ColorReset)
}

func PrintEvent(reason, message string, isWarning bool) {
	if !isTerminal() {
		if isWarning {
			fmt.Printf("│ ⚠️  [%s] %s\n", reason, message)
		} else {
			fmt.Printf("│ ℹ️  [%s] %s\n", reason, message)
		}
		return
	}

	if isWarning || reason == "HealthCheckFailed" || reason == "DependencyNotReady" {
		fmt.Printf("%s│ %s⚠️  [%s] %s%s\n", ColorSubLog, ColorYellow, reason, message, ColorReset)
	} else {
		fmt.Printf("%s│ ℹ️  [%s] %s\n", ColorSubLog, reason, message)
	}
}

func PrintMain(emoji, message string, color string) {
	if !isTerminal() {
		fmt.Printf("%s %s\n", emoji, message)
		return
	}
	fmt.Printf("%s%s%s %s%s\n", color, emoji, ColorReset, message, ColorReset)
}

func PrintWarning(message string) {
	if !isTerminal() {
		fmt.Printf("│ ⚠️  %s\n", message)
		return
	}
	fmt.Printf("%s│ %s⚠️  %s%s\n", ColorSubLog, ColorYellow, message, ColorReset)
}
