package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Cogsworth struct {
	store      Store
	runtime    Runtime
	reconciler *Reconciler
	scheduler  *Scheduler

	//multi-node fields
	nodeID    string
	role      NodeRole
	apiServer *APIServer
	apiClient *APIClient
}

func NewControlPlane(storePath, apiAddr string) (*Cogsworth, error) {
	store, err := NewBoltStore(storePath)
	if err != nil {
		return nil, err
	}

	cogs := &Cogsworth{
		store:     store,
		scheduler: NewScheduler(store),
		nodeID:    "control-plane-1",
		role:      ControlPlane,
		apiServer: NewAPIServer(store, apiAddr),
	}

	cogs.reconciler = NewReconciler(cogs, 5*time.Second)
	return cogs, nil
}

func NewWorkerNode(nodeID, controlPlaneURL string) (*Cogsworth, error) {
	runtime, err := NewDockerRuntime()
	if err != nil {
		return nil, err
	}

	cogs := &Cogsworth{
		runtime:   runtime,
		nodeID:    nodeID,
		role:      Worker,
		apiClient: NewAPIClient(controlPlaneURL, nodeID),
	}

	cogs.reconciler = NewReconciler(cogs, 5*time.Second)
	return cogs, nil
}

func NewCogsworth(store Store, runtime Runtime) *Cogsworth {
	c := &Cogsworth{
		store:   store,
		runtime: runtime,
	}

	c.reconciler = NewReconciler(c, 5*time.Second)

	return c
}

func (c *Cogsworth) CreateContainer(ctx context.Context, image string, ports []PortMapping) (*Container, error) {
	container := &Container{
		ID:           generateID(),
		Image:        image,
		State:        Requested,
		DesiredState: Running,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Ports:        ports,
		Env:          make(map[string]string),
		RestartCount: 0,
	}

	err := c.store.SaveContainer(ctx, container)
	if err != nil {
		return nil, err
	}

	container.State = Pulling
	c.store.SaveContainer(ctx, container)

	err = c.runtime.Pull(ctx, image)
	if err != nil {
		container.State = Failed
		c.store.SaveContainer(ctx, container)
		return nil, err
	}

	spec := &ContainerSpec{
		Image: image,
		Ports: ports,
		Env:   container.Env,
		Name:  container.ID,
	}

	dockerId, err := c.runtime.Create(ctx, spec)
	if err != nil {
		container.State = Failed
		c.store.SaveContainer(ctx, container)
		return nil, err
	}

	container.ContainerID = dockerId
	container.State = Created
	container.UpdatedAt = time.Now()
	c.store.SaveContainer(ctx, container)

	return container, nil
}

func (c *Cogsworth) StartContainer(ctx context.Context, id string) error {
	container, err := c.store.GetContainer(ctx, id)
	if err != nil {
		return err
	}

	if container.ContainerID == "" {
		return fmt.Errorf("container has no runtime ID")
	}

	if container.State == Running {
		return nil
	}

	if container.State != Created && container.State != Stopped {
		return fmt.Errorf("cannot start container in state: %s", container.State)
	}

	container.State = Starting
	c.store.SaveContainer(ctx, container)

	err = c.runtime.Start(ctx, container.ContainerID)
	if err != nil {
		container.State = Failed
		c.store.SaveContainer(ctx, container)
		return err
	}

	status, err := c.runtime.Inspect(ctx, container.ContainerID)
	if err == nil {
		container.IPAddress = status.IPAddress
	}

	container.State = Running
	container.UpdatedAt = time.Now()
	c.store.SaveContainer(ctx, container)

	return nil
}

// this does create and start together
func (c *Cogsworth) RunContainer(ctx context.Context, image string, ports []PortMapping) (*Container, error) {
	container, err := c.CreateContainer(ctx, image, ports)
	if err != nil {
		return nil, err
	}

	err = c.StartContainer(ctx, container.ID)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func (c *Cogsworth) RestartContainer(ctx context.Context, id string) error {
	err := c.StopContainer(ctx, id)
	if err != nil {
		return err
	}

	return c.StartContainer(ctx, id)
}

func (c *Cogsworth) StopContainer(ctx context.Context, id string) error {
	container, err := c.store.GetContainer(ctx, id)
	if err != nil {
		return err
	}

	if container.ContainerID == "" {
		return fmt.Errorf("container has no runtime ID")
	}

	container.State = Stopping
	c.store.SaveContainer(ctx, container)

	err = c.runtime.Stop(ctx, container.ContainerID, 10)
	if err != nil {
		container.State = Failed
		c.store.SaveContainer(ctx, container)
		return err
	}

	container.State = Stopped
	container.DesiredState = Stopped
	container.UpdatedAt = time.Now()
	c.store.SaveContainer(ctx, container)

	return nil
}

func (c *Cogsworth) DeleteContainer(ctx context.Context, id string) error {
	container, err := c.store.GetContainer(ctx, id)
	if err != nil {
		return nil
	}

	if container.ContainerID != "" {
		err = c.runtime.Remove(ctx, container.ContainerID)
		if err != nil {
			return err
		}
	}

	err = c.store.DelContainer(ctx, id)
	if err != nil {
		return err
	}

	return nil
}

func generateID() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return fmt.Sprintf("cont-%s", hex.EncodeToString(bytes))
}
