package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrDisabled = errors.New("connector disabled")
var ErrApprovalRequired = errors.New("approval required")

type Capability string

const (
	ReadDocuments Capability = "read_internal_documents"
	WriteWorkItem Capability = "write_work_item"
	SendMessage   Capability = "send_message"
)

type Config struct {
	Name, Endpoint, Token string
	Capabilities          []Capability
}
type Status struct {
	Name           string
	Enabled        bool
	Capabilities   []Capability
	DisabledReason string
}
type Delivery struct{ ExternalReference string }
type Registry struct {
	configs map[string]Config
	client  *http.Client
}

func New(configs []Config) *Registry {
	values := map[string]Config{}
	for _, config := range configs {
		values[config.Name] = config
	}
	return &Registry{configs: values, client: &http.Client{Timeout: 15 * time.Second}}
}
func (r *Registry) List() []Status {
	order := []string{"drive", "notion", "confluence", "github", "jira", "slack"}
	out := make([]Status, 0, len(order))
	for _, name := range order {
		config := r.configs[name]
		status := Status{Name: name, Capabilities: config.Capabilities, Enabled: config.Endpoint != "" && config.Token != ""}
		if !status.Enabled {
			status.DisabledReason = "configuration_missing"
		}
		out = append(out, status)
	}
	return out
}
func (r *Registry) Deliver(ctx context.Context, name, target, payload, approvalID string) (Delivery, error) {
	config, ok := r.configs[name]
	if !ok || config.Endpoint == "" || config.Token == "" {
		return Delivery{}, ErrDisabled
	}
	if requiresApproval(config.Capabilities) && strings.TrimSpace(approvalID) == "" {
		return Delivery{}, ErrApprovalRequired
	}
	body, _ := json.Marshal(map[string]string{"target": target, "payload": payload})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Delivery{}, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return Delivery{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Delivery{}, errors.New("connector delivery failed")
	}
	return Delivery{ExternalReference: response.Header.Get("Location")}, nil
}
func requiresApproval(values []Capability) bool {
	for _, value := range values {
		if value == WriteWorkItem || value == SendMessage {
			return true
		}
	}
	return false
}
