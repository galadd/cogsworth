package main

import (
	"context"
	"fmt"
	"log"
)

func (c *Cogsworth) StartReconciler(ctx context.Context) {
	go c.reconciler.Start(ctx)
}

func (c *Cogsworth) StopReconciler() {
	c.reconciler.Stop()
}

func cleanupAll(ctx context.Context, cogsworth *Cogsworth) {
	containers, _ := cogsworth.store.ListContainers(ctx)
	for _, c := range containers {
		if c.ContainerID != "" {
			cogsworth.runtime.Remove(ctx, c.ContainerID)
		}
		cogsworth.store.DelContainer(ctx, c.ID)
	}
}

func main() {
	fmt.Println("🕰️  Starting Cogsworth Orchestrator")

	store, _ := NewBoltStore("./cogsworth.db")
	defer store.Close()

	runtime, _ := NewDockerRuntime()
	defer runtime.Close()

	cogsworth := NewCogsworth(store, runtime)
	ctx := context.Background()

	fmt.Println("=== Cleaning Up Previous Containers ===")
	cleanupAll(ctx, cogsworth)

	cogsworth.StartReconciler(ctx)
	defer cogsworth.StopReconciler()

	fmt.Println("=== Creating Container ===")

	container, err := cogsworth.CreateContainer(ctx, "nginx:alpine", []PortMapping{
		{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== Starting Container ===")

	err = cogsworth.StartContainer(ctx, container.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nNginx running at http://localhost:8080")
	fmt.Println("\n=== Demo: Automatic Recovery ===")
	fmt.Println("http://localhost:8080")
	fmt.Println("docker kill <container-id>")
	fmt.Println("Wait 5-10 seconds")
	fmt.Println("\nPress Enter when done...")
	fmt.Scanln()

	cogsworth.StopContainer(ctx, container.ID)
	cogsworth.DeleteContainer(ctx, container.ID)
	fmt.Println("\nAutoRecovery complete!")
}
