package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Lists all currently running containers
func ps() error {
	dirPath := StateDir()
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("Error reading directory: %w", err)
	}

	// Reads each JSON in the directory and checks the Status field. Prints container ID, status, and the running commmand
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

// Stops the running process without removing the respective container JSON
func stop(cid string) error {
	container, err := loadContainer(cid)
	if err != nil {
		return fmt.Errorf("Cannot find container with ID %v: %w", cid, err)
	}
	process, err := os.FindProcess(container.PID)
	if err != nil {
		return fmt.Errorf("Cannot find process with PID %v: %w", container.PID, err)
	}
	// Signals to terminate the process. Checks if it has stopped every second. If it hasn't stopped after 10 seconds, kill it.
	process.Signal(syscall.SIGTERM)
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			break
		}
		if i == 9 {
			process.Signal(syscall.SIGKILL)
		}
	}

	// Update container Status in JSON
	container.Status = "stopped"
	saveContainerState(container)

	return nil
}

// Destructs container JSON for given stopped container
func remove(cid string) error {
	// Check if container is stopped
	container, err := loadContainer(cid)
	if err != nil {
		return fmt.Errorf("Cannot find container with ID %v: %w", cid, err)
	}
	if container.Status != "stopped" {
		return fmt.Errorf("Container is not yet stopped")
	}
	// Remove JSON file and unmount cgroup
	cleanupContainer(cid)
	cleanupCgroup(cid)
	path := filepath.Join(StateDir(), cid+".json")
	os.Remove(path)

	return nil
}
