# Mocker - Linux Container Runtime in Go
A simple container runtime inspired by Docker using Go and Linux namespaces. Built to better understand how containers works.

## Features
- Process isolation via Linux namespaces (PID, UTS, mount, network, IPC)
- Layered filesystems using OverlayFS (multiple containers share a base image without copying)
- Resource limiting via cgroups v2 (memory and CPU)
- Container lifecycle management (run, ps, stop, rm)
- Persistent container state storage using JSON
- Alpine Linux base image


# Getting Started

### Prerequisites
- Linux kernel 5.x+ (WSL2 works)
- Go 1.21+
- Root privileges

### Installation
```bash
git clone https://github.com/gweinrich/mocker.git
cd mocker
go build -o mocker ./cmd/Mocker/
```

## Usage
### Run a container
```bash
# Syntax
sudo -E ./mocker run <command>

# Example
sudo -E ./mocker run /bin/sh
```

### Run with resource limits
```bash
# Syntax
sudo -E ./mocker run [--memory <mb>] [--cpu <percent>] <command>

# Example
sudo -E ./mocker run --memory 128 --cpu 50 /bin/sh
```

### List running containers
```bash
sudo -E ./mocker ps
```

### Stop a container
```bash
sudo -E ./mocker stop <id>
```

### Remove a container
```bash
sudo -E ./mocker rm <id>
```

## How It Works

### Namespaces
In order to isolate each container, I had to learn how to use Linux namespaces.
Namespaces separate what resources processes can see and use, allowing them to act 
independently from one another on the same machine. The namespaces I made use of are as follows:
- PID: controls process ID denotion by creating a process tree for the container, marking the first process with PID 1.
This prevents the container from interfering with other processes running on the machine.
- UTS: the UNIX Timesharing System namespace isolates the hostname so that containers have distinct names from the host system.
- Mount: allows filesystems to be mounted without affecting the host filesystem. 
This means that the container can have an isolated filesystem that doesn't have access to the host's files.
- Network: establishes an independent routing table and IP address for the container.
- IPC: the Inter-Process Communication namespace isolates shared process resources, such as shared memory and message queues, 
thereby preventing containers from interfering with one another.

### OverlayFS
OverlayFS is used to partition the filesystem into a read-only lower directory and a read-write upper directory. 
The OS image, Alpine Linux, is the lower, read-only directory. By making it read-only, all active containers 
can use it as their underlying OS image without worrying about altering it and affecting other containers.
Additionally, this means that only one instance of Alpine Linux has to be initialized at a time, reducing start up overhead.
The upper directory stores the container specific filesystem that can only be edited by the respective container, thereby isolating it.

### cgroup v2
Control Group v2 is a Linux kernel feature that I learned to use in order to limit container resource usage.
The parent process establishes a cgroup, which is a directory that stores a control files under /sys/fs/cgroup. These file define the limits for CPU and memory usage.
As child processes are created, their PIDs are added to the cgroup.procs folder in the directory, thereby applying the same resource limitation rules.

## Limitations/Future Work
- No container networking: containers are network isolated but have no external connectivity.
- Requires root privileges: rootless containers with user namespaces are needed to avoid cgroup elevated permission requirements.
- Single base image: only Alpine is currently supported until dynamic image pulling is implemented.
