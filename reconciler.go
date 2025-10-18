package main

import (
	"context"
	"fmt"
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
			r.reconcile(ctx)
		case <-r.stopCh:
			fmt.Println("Stopping reconciliation loop")
			return
		case <-ctx.Done():
			fmt.Println("context cancelled, stopping reconciliation")
			return
		}
	}

}

func (r *Reconciler) Stop() {
	close(r.stopCh)
}

func (r *Reconciler) reconcile(ctx context.Context) {
	containers, err := r.cogsworth.store.ListContainers(ctx)
	if err != nil {
		fmt.Printf("Reconcile error: %v\n", err)
		return
	}

	for _, container := range containers {
		err := r.reconcileContainer(ctx, container)
		if err != nil {
			fmt.Printf("Reconcile error: %v\n", err)
		}
	}
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
		r.cogsworth.store.SaveContainer(ctx, container)
	}

	if actualState != Running {
		err := r.cogsworth.runtime.Start(ctx, container.ContainerID)
		if err != nil {
			container.RestartCount++
			container.State = Failed
			r.cogsworth.store.SaveContainer(ctx, container)

			// give up after 3 restarts
			if container.RestartCount >= 3 {
				return fmt.Errorf("max restart: container %s failed %d times, giving up\n", container.ID, container.RestartCount)
			}

			return err
		}

		status, _ := r.cogsworth.runtime.Inspect(ctx, container.ContainerID)
		if status != nil {
			container.IPAddress = status.IPAddress
		}

		container.State = Running
		container.UpdatedAt = time.Now()
		r.cogsworth.store.SaveContainer(ctx, container)
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
		r.cogsworth.store.SaveContainer(ctx, container)
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

	err := r.cogsworth.store.DelContainer(ctx, container.ID)
	if err != nil {
		return err
	}

	return nil
}
