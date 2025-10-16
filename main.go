package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.etcd.io/bbolt"
)

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

type ContainerState string

const (
	Requested  ContainerState = "requested"
	Scheduling ContainerState = "scheduling"
	Scheduled  ContainerState = "scheduled"
	Pulling    ContainerState = "pulling"
	Starting   ContainerState = "starting"
	Running    ContainerState = "running"
	Stopping   ContainerState = "stopping"
	Stopped    ContainerState = "stopped"
	Failed     ContainerState = "failed"
	Destroyed  ContainerState = "destroyed"
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

type NodeStore interface {
	SaveContainer(ctx context.Context, c *Container) error
	GetContainer(ctx context.Context, id string) (*Container, error)
	ListContainers(ctx context.Context) ([]*Container, error)
	DelContainer(ctx context.Context, id string) error

	CloseDB() error
}

type BoltStore struct {
	db *bbolt.DB
}

var containersBucket = []byte("containers")

func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(containersBucket)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}

	return &BoltStore{db: db}, nil
}

func (s *BoltStore) SaveContainer(ctx context.Context, c *Container) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(containersBucket)
		if bucket == nil {
			return fmt.Errorf("container's bucket not found")
		}

		data, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("failed to marshal container: %w", err)
		}

		// key = containerID, value = JSON bytes
		err = bucket.Put([]byte(c.ID), data)
		if err != nil {
			return fmt.Errorf("failed to save container: %w", err)
		}

		return nil
	})
}

func (s *BoltStore) GetContainer(ctx context.Context, id string) (*Container, error) {
	var container *Container

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(containersBucket)
		if bucket == nil {
			return fmt.Errorf("containers bucket not found")
		}

		data := bucket.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("containers %s not found", id)
		}

		container = &Container{}
		err := json.Unmarshal(data, container)
		if err != nil {
			return fmt.Errorf("failed to unmarshal container: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return container, nil
}

func (s *BoltStore) ListContainers(ctx context.Context) ([]*Container, error) {
	var containers []*Container

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(containersBucket)
		if bucket == nil {
			return fmt.Errorf("containers bucket not found")
		}

		return bucket.ForEach(func(k, v []byte) error {
			var container Container
			err := json.Unmarshal(v, &container)
			if err != nil {
				return fmt.Errorf("failed to unmarshal container: %w", err)
			}

			containers = append(containers, &container)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return containers, nil
}

func (s *BoltStore) DelContainer(ctx context.Context, id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(containersBucket)
		if bucket == nil {
			return fmt.Errorf("container's bucket not found")
		}

		return bucket.Delete([]byte(id))
	})
}
