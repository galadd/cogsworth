# Cogsworth
A distributed container orchestrator that schedules and manages containers across multiple nodes. Built with Go, BoltDB, and Docker, Cogsworth features a control plane for scheduling decisions and worker nodes that execute container workloads, similar to Kubernetes but simplified for learning and experimentation.

### Features
* Multi-node orchestration: Deploy containers across multiple worker nodes  
* Intelligent scheduling: Automatically assigns containers to least-loaded nodes  
* Self-healing: Automatically restarts failed containers (up to 3 attempts)  
* Node health monitoring: Detects unhealthy nodes and reschedules their containers  
* Reconciliation loop: Continuously ensures actual state matches desired state  
* REST API: Control plane exposes HTTP API for cluster management  
* Persistent state: Uses BoltDB to store container and node metadata  

## Installation

### Build from source
```bash
git clone https://github.com/galadd/cogsworth
cd cogsworth
go build -o cogs .
```

### Run
1. Start the controller plane  
```bash
./cogs start-control
```

**Output:**
```
Cogsworth Control Plane Starting...
API Server listening on :8080
Control plane node ID: control-plane-1
Starting reconciliation loop (interval: 5s)
```

2. Start Worker nodes  
In separate terminals, start on one or more worker nodes
```bash
# Terminal 2: Worker Node 1
./cogs start-worker http://localhost:8080

# Terminal 3: Worker Node 2
./cogs start-worker http://localhost:8080
```

```bash
# to list all nodes
./cogs nodes
```

```bash
# to add a container
./cogs add <image> <host_port>:<container_port>
```

```bash
# to delete a container
./cogs rm <container_id>
```

```bash
# list all containers
./cogs ls
```

```bash
# clean up all
./cogs clean
```
