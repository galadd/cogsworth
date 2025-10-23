package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	usage := `Usage:
		./cogs start                           Start the daemon
		./cogs add <image> [host:container]    Add a container
		./cogs list                            List containers
		./cogs delete <id>                     Delete a container`

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
	case "start":
		start()
	case "add":
		addContainer()
	case "list", "ls":
		listContainers()
	case "delete", "rm":
		deleteContainer()
	case "clean":
		cleanupAll()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		fmt.Println(usage)
		fmt.Println("\n", examples)
		os.Exit(1)
	}
}

func (c *Cogsworth) StartReconciler(ctx context.Context) {
	go c.reconciler.Start(ctx)
}

func (c *Cogsworth) StopReconciler() {
	c.reconciler.Stop()
}

func start() {
	dbPath := flag.String("db", "./cogsworth.db", "Database path")
	flag.Parse()

	fmt.Println("Cogsworth")
	fmt.Printf("Database: %s\n", *dbPath)

	store, err := NewBoltStore(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	runtime, err := NewDockerRuntime()
	if err != nil {
		log.Fatal(err)
	}
	defer runtime.Close()

	cogsworth := NewCogsworth(store, runtime)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Staring reconciliation loop (every 5s)...")
	fmt.Println("Watching database for changes...")
	fmt.Println("Press Ctrl-C to stop")

	cogsworth.StartReconciler(ctx)
	defer cogsworth.StopReconciler()

	select {}
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
			ports = []PortMapping{{host, container, "tcp"}}
		}
	}

	store, err := NewBoltStore("./cogsworth.db")
	if err != nil {
		log.Fatal(err)
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
	}

	ctx := context.Background()
	err = store.SaveContainer(ctx, container)
	if err != nil {
		log.Fatal(err)
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
