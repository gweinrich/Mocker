package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Define Container struct for JSON files
type Container struct {
	ID      string   `json:"id"`
	PID     int      `json:"pid"`
	Status  string   `json:"status"`
	Command []string `json:"command"`
	Created string   `json:"created"`
}

// Re-run mocker but with "child" as the argument
func run(args []string) {
	containerID := generateID()

	// Set up cgroup before starting container
	if err := setupCgroup(containerID, memLimit, cpuPercent); err != nil {
		fmt.Println("Error setting up cgroup:", err)
		os.Exit(1)
	}

	// Define what code the container runs based on CLI input
	cmd := exec.Command("/proc/self/exe", append([]string{"child", containerID}, args...)...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Define which namespaces to create for child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC,
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Instantiate JSON for container state
	containerJSON := Container{
		ID:      containerID,
		PID:     cmd.Process.Pid,
		Status:  "running",
		Command: args,
		Created: time.Now().Format(time.RFC3339),
	}

	if err := saveContainerState(containerJSON); err != nil {
		fmt.Println("Error saving state:", err)
	}

	// Wait for container's code to complete, then cleanup the container
	cmd.Wait()

	containerJSON.Status = "stopped"
	saveContainerState(containerJSON)

	cleanupContainer(containerID)
	cleanupCgroup(containerID)
}

// Child process is created in the new namespace upon re-running mocker
// Args are passed from run()
func child() {
	// Set a custom hostname
	syscall.Sethostname([]byte("mocker-container"))

	containerID := os.Args[2]

	// Join the cgroup
	if err := addToCgroup(containerID, os.Getpid()); err != nil {
		fmt.Println("Error joining cgroup:", err)
		os.Exit(1)
	}

	if err := createContainerDirs(containerID); err != nil {
		fmt.Println("Error creating container dirs:", err)
		os.Exit(1)
	}

	if err := mountOverlay(containerID); err != nil {
		fmt.Println("Error mounting overlay:", err)
		os.Exit(1)
	}

	mergedDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "containers", containerID, "merged")

	// Set mount namespace so container has its own file system
	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")

	input, _ := os.ReadFile("/etc/resolv.conf")
	os.WriteFile(filepath.Join(mergedDir, "etc/resolv.conf"), input, 0644)

	// Pivot root to the container's merged directory
	syscall.Chroot(mergedDir)
	syscall.Chdir("/")

	// Mount /sys so CPU info is accessible
	syscall.Mount("sysfs", "/sys", "sysfs", 0, "")
	defer syscall.Unmount("/sys", syscall.MNT_DETACH)

	syscall.Mount("proc", "/proc", "proc", 0, "")
	defer syscall.Unmount("/proc", syscall.MNT_DETACH)

	// Run whatever command was passed in
	cmd := exec.Command(os.Args[3], os.Args[4:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Allows the user to interact with a shell inside the container via their terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
		Ctty:   0,
	}

	// Run the designated code
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// Create a unique string ID for the container, distinct from PID
func generateID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}
