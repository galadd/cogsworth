package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type APIServer struct {
	store Store
	addr  string
}

func NewAPIServer(store Store, addr string) *APIServer {
	return &APIServer{
		store: store,
		addr:  addr,
	}
}

func (s *APIServer) Start() error {
	http.HandleFunc("/nodes/register", func(w http.ResponseWriter, r *http.Request) {
		var node Node
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		node.LastSeen = time.Now()
		node.State = NodeReady

		if err := s.store.SaveNode(context.Background(), &node); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("Node registered: %s at %v\n", node.ID, node.Address)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/nodes/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var hb struct {
			NodeID string `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		node, err := s.store.GetNode(context.Background(), hb.NodeID)
		if err != nil {
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}

		node.LastSeen = time.Now()
		s.store.SaveNode(context.Background(), node)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/containers/assigned", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node_id")
		if nodeID == "" {
			http.Error(w, "node_id parameter required", http.StatusBadRequest)
			return
		}

		containers, err := s.store.ListContainers(context.Background())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		assigned := []*Container{}
		for _, c := range containers {
			if c.NodeID == nodeID && c.Scheduled {
				assigned = append(assigned, c)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(assigned)
	})

	http.HandleFunc("/containers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var container Container
			if err := json.NewDecoder(r.Body).Decode(&container); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if err := s.store.SaveContainer(context.Background(), &container); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"id":     container.ID,
				"status": "scheduled",
			})

		case http.MethodDelete:
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) < 3 {
				http.Error(w, "Container ID required", http.StatusBadRequest)
				return
			}
			containerID := parts[2]

			if err := s.store.DelContainer(context.Background(), containerID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			log.Printf("[API] Container deleted: %s", containerID)
			w.WriteHeader(http.StatusOK)
		}

	})

	http.HandleFunc("/containers/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var container Container
		if err := json.NewDecoder(r.Body).Decode(&container); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.store.SaveContainer(context.Background(), &container); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[API] Container status updated: %s -> %s", container.ID, container.State)
		w.WriteHeader(http.StatusOK)
	})

	return http.ListenAndServe(s.addr, nil)
}

type APIClient struct {
	controlPlaneURL string
	nodeID          string
	client          *http.Client
}

func NewAPIClient(controlPlaneURL string, nodeID string) *APIClient {
	return &APIClient{
		controlPlaneURL: controlPlaneURL,
		nodeID:          nodeID,
		client:          &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *APIClient) Register(node *Node) error {
	data, _ := json.Marshal(node)
	resp, err := c.client.Post(
		c.controlPlaneURL+"/nodes/register",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *APIClient) SendHeartbeat() error {
	data, _ := json.Marshal(map[string]string{
		"node_id": c.nodeID,
	})
	resp, err := c.client.Post(
		c.controlPlaneURL+"/nodes/heartbeat",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *APIClient) GetAssignedContainers(nodeID string) ([]*Container, error) {
	url := fmt.Sprintf("%s/containers/assigned?node_id=%s", c.controlPlaneURL, nodeID)

	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Failed to get containers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status: %v", err)
	}

	var containers []*Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("Failed to decode response: %v", err)
	}

	return containers, nil
}

func (c *APIClient) UpdateContainerStatus(container *Container) error {
	data, err := json.Marshal(container)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(
		c.controlPlaneURL+"/containers/status",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *APIClient) DeleteContainer(containerID string) error {
	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/containers/%s", c.controlPlaneURL, containerID),
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to delete container: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
