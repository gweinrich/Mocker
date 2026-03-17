package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "Mocker",
	Short:   "A minimal container runtime",
	Version: "0.1.0",
}

func main() {
	// "run" is the first argument passed to the program
	// e.g. "go run main.go run /bin/sh"
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	}
}

func run() {
	// Re-run this same program but with "child" as the argument
	// This is a common pattern for namespace setup
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// This is where the magic happens
	// CLONE_NEWUTS gives the child its own UTS namespace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC,
	}

	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func child() {
	// This code runs inside the new namespace
	// Set a custom hostname - this won't affect the host
	syscall.Sethostname([]byte("mocker-container"))

	// Set private file root
	// Set mount namespace so container has its own file system
	// Unmount once process is finished
	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
	syscall.Mount("proc", "/proc", "proc", 0, "")
	defer syscall.Unmount("/proc", syscall.MNT_DETACH)

	// Run whatever command was passed in
	// e.g. /bin/sh
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create a new session
		Ctty:   0,    // Set stdin (fd 0) as the controlling terminal
	}

	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
