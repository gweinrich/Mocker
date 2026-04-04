package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func saveContainerState(c Container) error {
	// Creates directory for container JSONs if it doesn't already exist
	dir := StateDir()
	os.MkdirAll(dir, 0755)

	// Formats Container struct to JSON
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	// Creates the container's JSON file if it doesn't already exist and writes to it
	path := filepath.Join(dir, c.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func loadContainer(containerID string) (Container, error) {
	// Reads the JSON file
	path := filepath.Join(StateDir(), containerID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Container{}, fmt.Errorf("container %s not found", containerID)
	}

	// Formats JSON data to Container struct
	var container Container
	err = json.Unmarshal(data, &container)
	return container, err
}

// Function for returning the directory as a string
func StateDir() string {
	return filepath.Join(os.Getenv("HOME"), ".mocker", "containers")
}
