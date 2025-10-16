package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

type Cogsworth struct {
	store NodeStore
}

func main() {
	store, err := NewBoltStore("./test.db")
	if err != nil {
		log.Fatal(err)
	}
	defer store.db.Close()

	ctx := context.Background()

	container1 := &Container{
		ID:           "nginx-1",
		Image:        "nginx:latest",
		State:        Requested,
		DesiredState: Running,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Env: map[string]string{
			"ENV": "production",
		},
		Ports: []PortMapping{
			{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
	}

	err = store.SaveContainer(ctx, container1)
	if err != nil {
		log.Fatal(err)
	}

	container, err := store.GetContainer(ctx, "nginx-1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Container ID: %s \n State: %s \n Image: %s", container.ID, container.State, container.Image)

	container.State = Running
	container.UpdatedAt = time.Now()
	err = store.SaveContainer(ctx, container)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Check new state: ", container.State)

	err = store.DelContainer(ctx, "nginx-1")
	if err != nil {
		log.Fatal(err)
	}

	containers, err := store.ListContainers(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("containers list check: ", containers)
}
