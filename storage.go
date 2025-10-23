package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type NodeStore interface {
	SaveContainer(ctx context.Context, c *Container) error
	GetContainer(ctx context.Context, id string) (*Container, error)
	ListContainers(ctx context.Context) ([]*Container, error)
	DelContainer(ctx context.Context, id string) error
	Close() error
}

type BoltStore struct {
	db   *bbolt.DB
	path string
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
	db.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}
	return &BoltStore{db: db, path: path}, nil
}

func (s *BoltStore) SaveContainer(ctx context.Context, c *Container) error {
	err := s.withDB(s.path, func(db *bbolt.DB) error {
		return db.Update(func(tx *bbolt.Tx) error {
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
	})

	return err
}

func (s *BoltStore) GetContainer(ctx context.Context, id string) (*Container, error) {
	var container *Container

	err := s.withDB(s.path, func(db *bbolt.DB) error {
		return db.View(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(containersBucket)
			if bucket == nil {
				return fmt.Errorf("container's bucket not found")
			}

			data := bucket.Get([]byte(id))
			if data == nil {
				return fmt.Errorf("container %s not found", id)
			}

			container = &Container{}
			err := json.Unmarshal(data, container)
			if err != nil {
				return fmt.Errorf("failed to unmarshal container: %w", err)
			}

			return nil
		})
	})

	return container, err
}

func (s *BoltStore) ListContainers(ctx context.Context) ([]*Container, error) {
	var containers []*Container

	err := s.withDB(s.path, func(db *bbolt.DB) error {
		return db.View(func(tx *bbolt.Tx) error {
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
	})

	return containers, err
}

func (s *BoltStore) DelContainer(ctx context.Context, id string) error {
	err := s.withDB(s.path, func(db *bbolt.DB) error {
		return db.Update(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(containersBucket)
			if bucket == nil {
				return fmt.Errorf("container's bucket not found")
			}

			return bucket.Delete([]byte(id))
		})
	})
	return err
}

func (s *BoltStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *BoltStore) withDB(path string, fn func(*bbolt.DB) error) error {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	defer db.Close()

	return fn(db)
}
