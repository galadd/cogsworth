package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"
)

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
