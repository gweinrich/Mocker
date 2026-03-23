package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	containerID := generateID()

	cmd := exec.Command("/proc/self/exe", append([]string{"child", containerID}, os.Args[2:]...)...)

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
	cleanupContainer("c1")
}

func child() {
	// This code runs inside the new namespace
	// Set a custom hostname - this won't affect the host
	syscall.Sethostname([]byte("mocker-container"))

	containerID := os.Args[2] // now passed from run()

	if err := createContainerDirs(containerID); err != nil {
		fmt.Println("Error creating container dirs:", err)
		os.Exit(1)
	}

	if err := mountOverlay(containerID); err != nil {
		fmt.Println("Error mounting overlay:", err)
		os.Exit(1)
	}

	// Set up the overlay filesystem
	createContainerDirs(containerID)
	mountOverlay(containerID)

	mergedDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "containers", containerID, "merged")

	// Set private file root
	// Set mount namespace so container has its own file system
	// Unmount once process is finished
	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")

	// Pivot root to the container's merged directory
	syscall.Chroot(mergedDir)
	syscall.Chdir("/")

	syscall.Mount("proc", "/proc", "proc", 0, "")
	defer syscall.Unmount("/proc", syscall.MNT_DETACH)

	// Run whatever command was passed in
	// e.g. /bin/sh
	cmd := exec.Command(os.Args[3], os.Args[4:]...)
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

func createContainerDirs(containerID string) error {
	base := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "containers", containerID)
	fmt.Println("Creating container dirs at:", base)

	for _, dir := range []string{"upper", "work", "merged"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0755); err != nil {
			return err
		}
	}
	return nil
}

func mountOverlay(containerID string) error {
	imageDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "images", "alpine")
	containerDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "containers", containerID)

	upper := filepath.Join(containerDir, "upper")
	work := filepath.Join(containerDir, "work")
	merged := filepath.Join(containerDir, "merged")

	// This is the equivalent of:
	// mount -t overlay overlay -o lowerdir=alpine,upperdir=upper,workdir=work merged
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", imageDir, upper, work)
	err := syscall.Mount("overlay", merged, "overlay", 0, opts)
	if err != nil {
		fmt.Println("OverlayFS mount error:", err)
		return err
	}
	return nil
}

func cleanupContainer(containerID string) {
	containerDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "containers", containerID)
	merged := filepath.Join(containerDir, "merged")

	syscall.Unmount(merged, syscall.MNT_DETACH)
	os.RemoveAll(containerDir)
}

func generateID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b) // e.g. "a3f9c2"
}
