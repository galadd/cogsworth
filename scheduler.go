package main

import (
	"context"
	"time"
)

type Scheduler struct {
	store Store
}

func NewScheduler(store Store) *Scheduler {
	return &Scheduler{
		store: store,
	}
}

func (s *Scheduler) Schedule(ctx context.Context, container *Container) error {
	if container.Scheduled {
		return nil
	}

	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return err
	}

	selected := s.selectNode(nodes)

	container.NodeID = selected.ID
	container.Scheduled = true
	container.UpdatedAt = time.Now()

	return s.store.SaveContainer(ctx, container)
}

func (s *Scheduler) selectNode(nodes []*Node) *Node {
	var selected *Node
	minContainers := int(^uint(0) >> 1)

	for _, node := range nodes {
		if node.State != NodeReady {
			continue
		}

		count := s.countContainersOnNode(node.ID)
		if count < minContainers {
			selected = node
			minContainers = count
		}
	}

	return selected
}

func (s *Scheduler) countContainersOnNode(nodeId string) int {
	count := 0
	containers, _ := s.store.ListContainers(context.Background())

	for _, c := range containers {
		if c.NodeID == nodeId && c.State == Running {
			count++
		}
	}

	return count
}
