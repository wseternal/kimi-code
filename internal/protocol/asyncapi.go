// Package protocol provides AsyncAPI 3.1.0 spec generation (Gap #91).
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AsyncAPISpec represents an AsyncAPI 3.1.0 specification.
type AsyncAPISpec struct {
	AsyncAPI string                    `json:"asyncapi"`
	Info     AsyncAPIInfo              `json:"info"`
	Channels map[string]AsyncAPIChannel `json:"channels,omitempty"`
	Operations map[string]AsyncAPIOperation `json:"operations,omitempty"`
	Components *AsyncAPIComponents     `json:"components,omitempty"`
}

// AsyncAPIInfo holds API metadata.
type AsyncAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// AsyncAPIChannel represents a channel.
type AsyncAPIChannel struct {
	Address     string              `json:"address"`
	Messages    map[string]AsyncAPIRef `json:"messages,omitempty"`
	Description string              `json:"description,omitempty"`
}

// AsyncAPIOperation represents an operation.
type AsyncAPIOperation struct {
	Action  string       `json:"action"` // "send" or "receive"
	Channel AsyncAPIRef  `json:"channel"`
	Messages []AsyncAPIRef `json:"messages,omitempty"`
	Summary string       `json:"summary,omitempty"`
}

// AsyncAPIRef is a JSON reference.
type AsyncAPIRef struct {
	Ref string `json:"$ref,omitempty"`
}

// AsyncAPIComponents holds reusable components.
type AsyncAPIComponents struct {
	Messages map[string]AsyncAPIMessage `json:"messages,omitempty"`
	Schemas  map[string]any             `json:"schemas,omitempty"`
}

// AsyncAPIMessage represents a message definition.
type AsyncAPIMessage struct {
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Payload     any    `json:"payload,omitempty"`
}

// WSOperationDef defines a WebSocket operation for AsyncAPI generation.
type WSOperationDef struct {
	Name        string `json:"name"`
	Direction   string `json:"direction"` // "client_to_server" or "server_to_client"
	Channel     string `json:"channel"`
	MessageType string `json:"message_type"`
	Description string `json:"description,omitempty"`
}

// GenerateAsyncAPI generates an AsyncAPI 3.1.0 spec from WebSocket operation definitions.
func GenerateAsyncAPI(title, version string, ops []WSOperationDef) *AsyncAPISpec {
	spec := &AsyncAPISpec{
		AsyncAPI: "3.1.0",
		Info: AsyncAPIInfo{
			Title:   title,
			Version: version,
		},
		Channels:   make(map[string]AsyncAPIChannel),
		Operations: make(map[string]AsyncAPIOperation),
		Components: &AsyncAPIComponents{
			Messages: make(map[string]AsyncAPIMessage),
		},
	}

	for _, op := range ops {
		channelKey := sanitizeKey(op.Channel)
		msgKey := sanitizeKey(op.MessageType)

		// Add channel
		if _, exists := spec.Channels[channelKey]; !exists {
			spec.Channels[channelKey] = AsyncAPIChannel{
				Address:     op.Channel,
				Description: fmt.Sprintf("Channel for %s", op.Channel),
				Messages:    make(map[string]AsyncAPIRef),
			}
		}
		ch := spec.Channels[channelKey]
		ch.Messages[msgKey] = AsyncAPIRef{Ref: "#/components/messages/" + msgKey}
		spec.Channels[channelKey] = ch

		// Add message component
		if _, exists := spec.Components.Messages[msgKey]; !exists {
			spec.Components.Messages[msgKey] = AsyncAPIMessage{
				Name:        op.MessageType,
				Title:       op.MessageType,
				ContentType: "application/json",
			}
		}

		// Add operation
		action := "send"
		if op.Direction == "server_to_client" {
			action = "receive"
		}
		spec.Operations[sanitizeKey(op.Name)] = AsyncAPIOperation{
			Action:  action,
			Channel: AsyncAPIRef{Ref: "#/channels/" + channelKey},
			Messages: []AsyncAPIRef{{Ref: "#/components/messages/" + msgKey}},
			Summary: op.Description,
		}
	}

	return spec
}

// GenerateAsyncAPIJSON generates the AsyncAPI spec as JSON.
func GenerateAsyncAPIJSON(title, version string, ops []WSOperationDef) (string, error) {
	spec := GenerateAsyncAPI(title, version, ops)
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal asyncapi: %w", err)
	}
	return string(data), nil
}

func sanitizeKey(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.TrimPrefix(s, "_")
	return s
}
