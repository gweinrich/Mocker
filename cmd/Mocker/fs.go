package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Create directories needed for OverlayFS for givern container
func createContainerDirs(containerID string) error {
	base := filepath.Join(StateDir(), containerID)

	for _, dir := range []string{"upper", "work", "merged"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0755); err != nil {
			return err
		}
	}
	return nil
}

// Mount the base image to the container as the lower directory
func mountOverlay(containerID string) error {
	imageDir := filepath.Join(os.Getenv("HOME"), "projects", "Mocker", "images", "alpine")
	containerDir := filepath.Join(StateDir(), containerID)

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

// Unmount the filesystem and delete the directory
func cleanupContainer(containerID string) {
	containerDir := filepath.Join(StateDir(), containerID)
	merged := filepath.Join(containerDir, "merged")

	syscall.Unmount(merged, syscall.MNT_DETACH)
	os.RemoveAll(containerDir)
}
