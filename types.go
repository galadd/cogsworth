package main

import "time"

type ContainerState string

const (
	Requested ContainerState = "requested"
	Pulling   ContainerState = "pulling"
	Created   ContainerState = "created"
	Starting  ContainerState = "starting"
	Running   ContainerState = "running"
	Stopping  ContainerState = "stopping"
	Stopped   ContainerState = "stopped"
	Failed    ContainerState = "failed"
	Destroyed ContainerState = "destroyed"
)

type Container struct {
	ID    string `json:"id"`
	Image string `json:"image"`

	State        ContainerState `json:"state"`
	DesiredState ContainerState `json:"desired_state"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ContainerID string `json:"container_id"`
	IPAddress   string `json:"ip_address"`

	Env   map[string]string `json:"env"`
	Ports []PortMapping     `json:"ports"`

	RestartCount int `json:"restart_count"`
}

type PortMapping struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type Node struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Role      NodeRole  `json:"role"`
	State     NodeState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`

	Capacity  Resources `json:"capacity,omitempty"`
	Allocated Resources `json:"allocated,omitempty"`
}

type NodeState string

const (
	NodeReady    NodeState = "ready"
	NodeNotReady NodeState = "not_ready"
)

type NodeRole string

const (
	ControlPlane NodeRole = "control-plane"
	Worker       NodeRole = "worker"
)

type Resources struct {
	CPUCores int   `json:"cpu_cores"`
	MemoryMB int64 `json:"memory_mb"`
	DiskGB   int64 `json:"disk_gb"`
}
