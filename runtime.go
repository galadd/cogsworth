package main

import (
	"context"
	"fmt"
	"io"
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type Runtime interface {
	Pull(ctx context.Context, image string) error
	Create(ctx context.Context, spec *ContainerSpec) (string, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout int) error
	Remove(ctx context.Context, containerID string) error

	Inspect(ctx context.Context, containerID string) (*RuntimeStatus, error)
	List(ctx context.Context) ([]*RuntimeStatus, error)
	Logs(ctx context.Context, containerID string, tail int) (string, error)

	Close() error
}

type ContainerSpec struct {
	Image string
	Env   map[string]string
	Ports []PortMapping
	Name  string
}

type RuntimeStatus struct {
	ContainerID string
	State       string
	IPAddress   string
	StartedAt   string
	ExitCode    int
	Error       string
}

type DockerRuntime struct {
	cli *client.Client
}

func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerRuntime{cli: cli}, nil
}

func (d *DockerRuntime) Pull(ctx context.Context, image string) error {
	reader, err := d.cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull output: %w", err)
	}

	fmt.Printf("Pulled image: %s\n", image)
	return nil
}

func (d DockerRuntime) Create(ctx context.Context, spec *ContainerSpec) (string, error) {
	portBindings := network.PortMap{}
	exposedPorts := network.PortSet{}

	for _, pm := range spec.Ports {
		containerPort, err := network.ParsePort(fmt.Sprintf("%d/%s", pm.ContainerPort, pm.Protocol))
		if err != nil {
			return "", err
		}

		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = []network.PortBinding{
			{
				HostIP:   netip.MustParseAddr("0.0.0.0"),
				HostPort: fmt.Sprintf("%d", pm.HostPort),
			},
		}
	}

	env := []string{}
	for k, v := range spec.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	resp, err := d.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        spec.Image,
			Env:          env,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			PortBindings: portBindings,
		},
		nil,
		nil,
		spec.Name,
	)

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	fmt.Printf("Created container: %s\n", resp.ID[:12])
	return resp.ID, nil
}

func (d *DockerRuntime) Start(ctx context.Context, containerID string) error {
	err := d.cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Printf("Started container: %s\n", containerID[:12])
	return nil
}

func (d *DockerRuntime) Stop(ctx context.Context, containerID string, timeout int) error {
	err := d.cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout})
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	fmt.Printf("Stopped container: %s\n", containerID[:12])
	return nil
}

func (d *DockerRuntime) Remove(ctx context.Context, containerID string) error {
	err := d.cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	fmt.Printf("Removed container: %s\n", containerID[:12])
	return nil
}

func (d *DockerRuntime) Inspect(ctx context.Context, containerID string) (*RuntimeStatus, error) {
	info, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	status := &RuntimeStatus{
		ContainerID: info.ID,
		State:       info.State.Status,
		StartedAt:   info.State.StartedAt,
		ExitCode:    info.State.ExitCode,
	}

	if info.NetworkSettings != nil {
		for _, netConf := range info.NetworkSettings.Networks {
			if netConf != nil && netConf.IPAddress.String() != "" {
				status.IPAddress = netConf.IPAddress.String()
			}
		}
	}

	if info.State.Error != "" {
		status.Error = info.State.Error
	}

	return status, nil
}

func (d *DockerRuntime) List(ctx context.Context) ([]*RuntimeStatus, error) {
	containers, err := d.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	statuses := make([]*RuntimeStatus, 0, len(containers))
	for _, c := range containers {
		status := &RuntimeStatus{
			ContainerID: c.ID,
			State:       c.State,
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (d *DockerRuntime) Logs(ctx context.Context, containerID string, tail int) (string, error) {
	options := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}

	reader, err := d.cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer reader.Close()

	logs, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}

func (d *DockerRuntime) Close() error {
	if d.cli != nil {
		return d.cli.Close()
	}
	return nil
}
