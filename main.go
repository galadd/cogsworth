package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	usage := `Usage:
		./cogs start-control                    Start control plane
		./cogs start-worker <control-url>       Start worker node
		./cogs add <image> [host:container]     Add a container
		./cogs list                             List containers
		./cogs delete <id>                      Delete a container`

	examples := `Examples:
		./cogs start
		./cogs add nginx:alpine 8080:80
		./cogs list
		./cogs delete cont-abc123`

	if len(os.Args) < 2 {
		fmt.Println("Cogsworth - Container Orchestrator")

		fmt.Println("\n", usage)
		fmt.Println("\n", examples)
	}

	command := os.Args[1]

	switch command {
	case "start-control":
		startControl()
	case "start-worker":
		startWorker()
	case "add":
		addContainer()
	case "list", "ls":
		listContainers()
	case "delete", "rm":
		deleteContainer()
	case "nodes":
		listNodes()
	case "clean":
		cleanupAll()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		fmt.Println(usage)
		fmt.Println("\n", examples)
		os.Exit(1)
	}
}

func startControl() {
	fmt.Println("Cogsworth Control Plane Starting...")
	fmt.Println("API Server listening on :8080")
	fmt.Println("Control plane node ID: control-plane-1")
	fmt.Println("Start Reconciliation loop. Interval: 5s")

	apiAddr := ":8080"
	if len(os.Args) > 2 {
		apiAddr = os.Args[2]
	}

	cogs, err := NewControlPlane("./cogsworth.db", apiAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer cogs.store.Close()

	go cogs.apiServer.Start()
	cogs.reconciler.Start(context.Background())
}

func startWorker() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: ./cogs start-worker <control-url>")
	}

	controlUrl := os.Args[2]
	nodeID := fmt.Sprintf("worker-%s", generateID())

	cogs, err := NewWorkerNode(nodeID, controlUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer cogs.runtime.Close()

	node := &Node{
		ID:      nodeID,
		Address: getLocalIP(),
		Role:    Worker,
		State:   NodeReady,
	}
	if err := cogs.apiClient.Register(node); err != nil {
		log.Fatal("Failed to register with control", err)
	}

	// send heartbeat
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cogs.apiClient.SendHeartbeat()
		}
	}()

	cogs.reconciler.Start(context.Background())
}

func addContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: ./cogs add <image> [host:container]")
		os.Exit(1)
	}

	image := os.Args[2]
	var ports []PortMapping

	if len(os.Args) >= 4 {
		parts := strings.Split(os.Args[3], ":")
		if len(parts) == 2 {
			host, _ := strconv.Atoi(parts[0])
			container, _ := strconv.Atoi(parts[1])

			if host == 8080 {
				fmt.Println("Warning: Port 8080 may conflict with Congsworth Control Plane")
				fmt.Println("Consider using a different port (e.g., 8081:80)")
			}
			ports = []PortMapping{{host, container, "tcp"}}
		}
	}

	container := &Container{
		ID:           generateID(),
		Image:        image,
		State:        Requested,
		DesiredState: Running,
		Ports:        ports,
		Env:          make(map[string]string),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Scheduled:    false,
		NodeID:       "",
	}

	controlPlaneURL := "http://localhost:8080"

	data, err := json.Marshal(container)
	if err != nil {
		log.Fatal("Failed to marshal container:", err)
	}

	resp, err := http.Post(
		controlPlaneURL+"/containers",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		log.Fatal("Failed to add container: ", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("API error: %s", string(body))
	}

	fmt.Printf("Added container: %s\n", container.ID)
	fmt.Printf("Image: %s\n", image)
	if len(ports) > 0 {
		fmt.Printf("Ports: %d:%d\n", ports[0].HostPort, ports[0].ContainerPort)
	}
}

func listContainers() {
	store, err := NewBoltStore("./cogsworth.db")
	if err != nil {
		log.Fatal(err)
	}

	containers, err := store.ListContainers(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found")
		return
	}

	fmt.Printf("%-20s %-20s %-10s %-10s\n", "ID", "IMAGE", "STATE", "DESIRED")
	fmt.Println(strings.Repeat("-", 60))
	for _, c := range containers {
		fmt.Printf("%-20s %-20s %-10s %-10s\n",
			c.ID,
			c.Image,
			c.State,
			c.DesiredState,
		)
	}
}

func deleteContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: ./cogs delete <id>")
		os.Exit(1)
	}

	id := os.Args[2]

	store, err := NewBoltStore("./cogsworth.db")
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	container, err := store.GetContainer(ctx, id)
	if err != nil {
		log.Fatalf("Delete Container error: %v", err)
	}

	container.DesiredState = Destroyed
	err = store.SaveContainer(ctx, container)
	if err != nil {
		log.Fatalf("Delete Container error: %v", err)
	}
}

func listNodes() {
	store, err := NewBoltStore("./cogsworth.db")
	if err != nil {
		log.Fatal(err)
	}

	nodes, err := store.ListNodes(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if len(nodes) == 0 {
		fmt.Println("No nodes found")
		return
	}

	fmt.Printf("%-30s %-15s %-10s %-10s\n", "ID", "ADDRESS", "ROLE", "STATE")
	fmt.Println(strings.Repeat("-", 65))
	for _, node := range nodes {
		fmt.Printf("%-30s %-15s %-10s %-10s\n",
			node.ID,
			node.Address,
			node.Role,
			node.State,
		)
	}
}

func cleanupAll() {
	store, err := NewBoltStore("./cogsworth.db")
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	containers, _ := store.ListContainers(ctx)
	for _, c := range containers {
		c.DesiredState = Destroyed
		_ = store.SaveContainer(ctx, c)
	}
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			log.Printf("Failed to get local ip: %v", err)
			return "127.0.0.1"
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
