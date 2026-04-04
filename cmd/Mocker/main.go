package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// CLI command definitions
var memLimit int
var cpuPercent int

var rootCmd = &cobra.Command{
	Use:     "Mocker",
	Short:   "A minimal container runtime",
	Version: "0.1.0",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a container",
	Run: func(cmd *cobra.Command, args []string) {
		run(args)
	},
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List all running containers",
	Run: func(cmd *cobra.Command, args []string) {
		if err := ps(); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stops the given container if running",
	Run: func(cmd *cobra.Command, args []string) {
		if err := stop(args[0]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	},
}

var rmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Removes the given stopped container",
	Run: func(cmd *cobra.Command, args []string) {
		if err := remove(args[0]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().IntVar(&memLimit, "memory", 0, "Memory limit in MB")
	runCmd.Flags().IntVar(&cpuPercent, "cpu", 0, "CPU limit as percentage")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(rmCmd)
}

func main() {
	if os.Getuid() != 0 {
		fmt.Println("Error: mocker must be run as root. Try sudo -E ./mocker")
		os.Exit(1)
	}

	// child is called internally by run() via /proc/self/exe re-execution.
	// It is not a user-facing command and intentionally bypasses Cobra.
	if len(os.Args) > 1 && os.Args[1] == "child" {
		child()
		return
	}

	// Run the given CLI command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
