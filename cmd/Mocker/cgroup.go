package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Create parent cgroup directory and apply controls
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

// Container and its PID to the cgroup
func addToCgroup(containerID string, pid int) error {
	cgPath := filepath.Join("/sys/fs/cgroup", "mocker-"+containerID)
	procsFile := filepath.Join(cgPath, "cgroup.procs")
	return os.WriteFile(procsFile, []byte(fmt.Sprintf("%d", pid)), 0700)
}

// Remove the cgroup directory if it exists
func cleanupCgroup(containerID string) error {
	cgPath := filepath.Join("/sys/fs/cgroup", "mocker-"+containerID)
	if _, err := os.Stat(cgPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(cgPath)
}
