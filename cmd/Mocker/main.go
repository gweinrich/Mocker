package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var memLimit int
var cpuPercent int

type Container struct {
	ID      string   `json:"id"`
	PID     int      `json:"pid"`
	Status  string   `json:"status"`
	Command []string `json:"command"`
	Created string   `json:"created"`
}

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

	if len(os.Args) > 1 && os.Args[1] == "child" {
		child()
		return
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(args []string) {
	// Re-run this same program but with "child" as the argument
	// This is a common pattern for namespace setup
	containerID := generateID()

	// Set up cgroup before starting container
	// For now hardcode limits, CLI flags come later
	if err := setupCgroup(containerID, memLimit, cpuPercent); err != nil {
		fmt.Println("Error setting up cgroup:", err)
		os.Exit(1)
	}

	cmd := exec.Command("/proc/self/exe", append([]string{"child", containerID}, args...)...)

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

	if err := cmd.Start(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

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

	cmd.Wait()

	containerJSON.Status = "stopped"
	saveContainerState(containerJSON)

	cleanupContainer(containerID)
	cleanupCgroup(containerID)
}

func child() {
	// This code runs inside the new namespace
	// Set a custom hostname - this won't affect the host
	syscall.Sethostname([]byte("mocker-container"))

	containerID := os.Args[2] // now passed from run()

	// Join the cgroup immediately
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

	// Set private file root
	// Set mount namespace so container has its own file system
	// Unmount once process is finished
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

func saveContainerState(c Container) error {
	dir := filepath.Join(os.Getenv("HOME"), ".mocker", "containers")
	os.MkdirAll(dir, 0755)

	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, c.ID+".json")
	return os.WriteFile(path, data, 0644)
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

func setupCgroup(containerID string, memLimitMB int, cpuPercent int) error {
	// Enable memory and cpu controllers in the parent cgroup
	err := os.WriteFile("/sys/fs/cgroup/cgroup.subtree_control",
		[]byte("+memory +cpu"), 0700)
	if err != nil {
		fmt.Println("Warning: could not enable controllers:", err)
	}

	cgPath := filepath.Join("/sys/fs/cgroup", "mocker-"+containerID)

	// Create the cgroup directory
	if err := os.MkdirAll(cgPath, 0755); err != nil {
		return fmt.Errorf("creating cgroup: %w", err)
	}

	// Set memory limit (convert MB to bytes)
	if memLimitMB > 0 {
		memBytes := int64(memLimitMB) * 1024 * 1024
		memFile := filepath.Join(cgPath, "memory.max")
		if err := os.WriteFile(memFile, []byte(fmt.Sprintf("%d", memBytes)), 0700); err != nil {
			return fmt.Errorf("setting memory limit: %w", err)
		}
	}

	// Set CPU limit
	if cpuPercent > 0 {
		cpuFile := filepath.Join(cgPath, "cpu.max")
		quota := cpuPercent * 1000
		if err := os.WriteFile(cpuFile, []byte(fmt.Sprintf("%d 100000", quota)), 0700); err != nil {
			return fmt.Errorf("setting cpu limit: %w", err)
		}
	}

	return nil
}

func addToCgroup(containerID string, pid int) error {
	cgPath := filepath.Join("/sys/fs/cgroup", "mocker-"+containerID)
	procsFile := filepath.Join(cgPath, "cgroup.procs")
	return os.WriteFile(procsFile, []byte(fmt.Sprintf("%d", pid)), 0700)
}

func cleanupCgroup(containerID string) error {
	cgPath := filepath.Join("/sys/fs/cgroup", "mocker-"+containerID)
	if _, err := os.Stat(cgPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(cgPath)
}

func ps() error {
	dirPath := filepath.Join(os.Getenv("HOME"), ".mocker", "containers")
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("Error reading directory: %w", err)
	}

	fmt.Printf("%-12s %-10s %-20s\n", "ID", "STATUS", "COMMAND")
	for _, entry := range entries {
		container := Container{}
		data, _ := os.ReadFile(filepath.Join(dirPath, entry.Name()))
		json.Unmarshal(data, &container)
		if container.Status == "running" {
			fmt.Printf("%-12s %-10s %-20s\n", container.ID, container.Status, strings.Join(container.Command, " "))
		}
	}

	return nil
}

func stop(cid string) error {
	container, err := loadContainer(cid)
	if err != nil {
		return fmt.Errorf("Cannot find container with ID %v: %w", cid, err)
	}
	process, err := os.FindProcess(container.PID)
	if err != nil {
		return fmt.Errorf("Cannot find process with PID %v: %w", container.PID, err)
	}
	process.Signal(syscall.SIGTERM)
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// process is gone, no need to SIGKILL
			break
		}
		if i == 9 {
			process.Signal(syscall.SIGKILL)
		}
	}

	container.Status = "stopped"
	saveContainerState(container)

	return nil
}

func remove(cid string) error {
	container, err := loadContainer(cid)
	if err != nil {
		return fmt.Errorf("Cannot find container with ID %v: %w", cid, err)
	}
	if container.Status != "stopped" {
		return fmt.Errorf("Container is not yet stopped")
	}
	cleanupContainer(cid)
	cleanupCgroup(cid)
	path := filepath.Join(os.Getenv("HOME"), ".mocker", "containers", cid+".json")
	os.Remove(path)

	return nil
}

func loadContainer(containerID string) (Container, error) {
	path := filepath.Join(os.Getenv("HOME"), ".mocker", "containers", containerID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Container{}, fmt.Errorf("container %s not found", containerID)
	}

	var container Container
	err = json.Unmarshal(data, &container)
	return container, err
}
