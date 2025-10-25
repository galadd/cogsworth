package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

type Reconciler struct {
	cogsworth *Cogsworth
	interval  time.Duration
	stopCh    chan struct{}
}

func NewReconciler(cogsworth *Cogsworth, interval time.Duration) *Reconciler {
	return &Reconciler{
		cogsworth: cogsworth,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

func (r *Reconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reconcile(ctx)

	for {
		select {
		case <-ticker.C:
			if err := r.reconcile(ctx); err != nil {
				log.Printf("Reconcile error: %v", err)
			}
		case <-r.stopCh:
			fmt.Println("Stopping reconciliation loop")
			return
		case <-ctx.Done():
			fmt.Println("context cancelled, stopping reconciliation")
			return
		}
	}

}

func (r *Reconciler) reconcile(ctx context.Context) error {
	if r.cogsworth.role == ControlPlane {
		return r.reconcileControlPlane(ctx)
	}

	return r.reconcileWorker(ctx)
}

func (r *Reconciler) reconcileControlPlane(ctx context.Context) error {
	containers, _ := r.cogsworth.store.ListContainers(ctx)
	for _, container := range containers {
		if !container.Scheduled && container.DesiredState == Running {
			r.cogsworth.scheduler.Schedule(ctx, container)
		}
	}

	nodes, _ := r.cogsworth.store.ListNodes(ctx)
	for _, node := range nodes {
		if time.Since(node.LastSeen) > 30*time.Second {
			log.Printf("Node %s is unhealthy, marking as NotReady\n", node.ID)
			node.State = NodeNotReady
			r.cogsworth.store.SaveNode(ctx, node)
		}
	}

	return nil
}

func (r *Reconciler) reconcileWorker(ctx context.Context) error {
	containers, err := r.cogsworth.apiClient.GetAssignedContainers(r.cogsworth.nodeID)
	if err != nil {
		log.Printf("Failed to fetch containers from control plane: %v", err)
		return err
	}

	for _, container := range containers {
		if container.NodeID != r.cogsworth.nodeID {
			continue
		}

		if err := r.reconcileContainer(ctx, container); err != nil {
			log.Printf("Failed to reconcile container %s: %v", container.ID, err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileContainer(ctx context.Context, container *Container) error {
	var actualState ContainerState
	var runtimeExists bool

	if container.ContainerID != "" {
		status, err := r.cogsworth.runtime.Inspect(ctx, container.ContainerID)
		if err != nil {
			runtimeExists = false
			actualState = ""
		} else {
			runtimeExists = true

			switch status.State {
			case "running":
				actualState = Running
			case "exited", "dead":
				actualState = Stopped
			case "created":
				actualState = Created
			default:
				actualState = Failed
			}
		}
	} else {
		runtimeExists = false
	}

	switch container.DesiredState {
	case Running:
		return r.reconcileRunning(ctx, container, actualState, runtimeExists)
	case Stopped:
		return r.reconcileStopped(ctx, container, actualState, runtimeExists)
	case Destroyed:
		return r.reconcileDestroyed(ctx, container, runtimeExists)
	}

	return nil
}

func (r *Reconciler) reconcileRunning(ctx context.Context, container *Container, actualState ContainerState, exists bool) error {
	if container.RestartCount >= 3 {
		return nil
	}
	if !exists {
		fmt.Printf("Container %s is missing, recreating...\n", container.ID)

		err := r.cogsworth.runtime.Pull(ctx, container.Image)
		if err != nil {
			return err
		}

		spec := &ContainerSpec{
			Image: container.Image,
			Ports: container.Ports,
			Env:   container.Env,
			Name:  container.ID,
		}

		dockerID, err := r.cogsworth.runtime.Create(ctx, spec)
		if err != nil {
			return err
		}

		container.ContainerID = dockerID
		container.State = Created

		r.saveContainerStatus(ctx, container)
	}

	if actualState != Running {
		err := r.cogsworth.runtime.Start(ctx, container.ContainerID)
		if err != nil {
			container.RestartCount++
			container.UpdatedAt = time.Now()

			if container.RestartCount >= 3 {
				fmt.Printf("Max restart: container %s failed %d times, giving up\n", container.ID, container.RestartCount)
				container.State = Failed
				container.DesiredState = Stopped
			} else {
				container.State = Failed
			}

			r.saveContainerStatus(ctx, container)
			return err
		}

		status, _ := r.cogsworth.runtime.Inspect(ctx, container.ContainerID)
		if status != nil {
			container.IPAddress = status.IPAddress
		}

		container.State = Running
		container.UpdatedAt = time.Now()

		r.saveContainerStatus(ctx, container)
	}

	return nil
}

func (r *Reconciler) reconcileStopped(ctx context.Context, container *Container, actualState ContainerState, exists bool) error {
	if exists && actualState == Running {
		err := r.cogsworth.runtime.Stop(ctx, container.ContainerID, 10)
		if err != nil {
			return err
		}

		container.State = Stopped
		container.UpdatedAt = time.Now()
		r.saveContainerStatus(ctx, container)
	}

	return nil
}

func (r *Reconciler) reconcileDestroyed(ctx context.Context, container *Container, exists bool) error {
	if exists {
		err := r.cogsworth.runtime.Remove(ctx, container.ContainerID)
		if err != nil {
			return err
		}
	}

	r.deleteContainer(ctx, container.ID)

	return nil
}

func (r *Reconciler) saveContainerStatus(ctx context.Context, container *Container) {
	if r.cogsworth.role == Worker {
		if err := r.cogsworth.apiClient.UpdateContainerStatus(container); err != nil {
			log.Printf("Failed to report status: %v", err)
		}
	} else {
		if err := r.cogsworth.store.SaveContainer(ctx, container); err != nil {
			log.Printf("Failed to save container: %v", err)
		}
	}
}

func (r *Reconciler) deleteContainer(ctx context.Context, containerID string) {
	if r.cogsworth.role == Worker {
		if err := r.cogsworth.apiClient.DeleteContainer(containerID); err != nil {
			log.Printf("Failed to notify control plane of deletion: %v", err)
		}
	} else {
		if err := r.cogsworth.store.DelContainer(ctx, containerID); err != nil {
			log.Printf("Failed to delete container: %v", err)
		}
	}
}
