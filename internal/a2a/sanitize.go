package a2a

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

const defaultMaxArtifactPartBytes = 256 * 1024

type SafePart struct {
	Text      *string         `json:"text,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Filename  string          `json:"filename,omitempty"`
	MediaType string          `json:"media_type,omitempty"`
}

type SafeArtifact struct {
	ArtifactID  string     `json:"artifact_id"`
	Name        string     `json:"name,omitempty"`
	Description string     `json:"description,omitempty"`
	Parts       []SafePart `json:"parts"`
}

type QuarantinedPart struct {
	ArtifactID string `json:"artifact_id"`
	Filename   string `json:"filename,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	SHA256     string `json:"sha256,omitempty"`
}

type TaskOutput struct {
	ProtocolVersion string            `json:"protocol_version"`
	TaskID          string            `json:"task_id"`
	ContextID       string            `json:"context_id,omitempty"`
	State           TaskState         `json:"state"`
	Artifacts       []SafeArtifact    `json:"artifacts,omitempty"`
	Quarantined     []QuarantinedPart `json:"quarantined,omitempty"`
}

type MessageOutput struct {
	ProtocolVersion string            `json:"protocol_version"`
	MessageID       string            `json:"message_id"`
	Parts           []SafePart        `json:"parts,omitempty"`
	Quarantined     []QuarantinedPart `json:"quarantined,omitempty"`
}

func SanitizeTask(task Task, maxPartBytes int) TaskOutput {
	if maxPartBytes <= 0 {
		maxPartBytes = defaultMaxArtifactPartBytes
	}
	output := TaskOutput{
		ProtocolVersion: ProtocolVersion,
		TaskID:          task.ID,
		ContextID:       task.ContextID,
		State:           task.Status.State,
	}
	for _, artifact := range task.Artifacts {
		safe := SafeArtifact{ArtifactID: artifact.ArtifactID, Name: artifact.Name, Description: artifact.Description}
		for _, part := range artifact.Parts {
			switch {
			case part.URL != "":
				output.Quarantined = append(output.Quarantined, QuarantinedPart{
					ArtifactID: artifact.ArtifactID, Filename: part.Filename, MediaType: part.MediaType,
					Kind: "url", Reason: "remote file URLs are disabled",
				})
			case part.Raw != "":
				raw, err := base64.StdEncoding.DecodeString(part.Raw)
				if err != nil {
					raw = []byte(part.Raw)
				}
				digest := sha256.Sum256(raw)
				output.Quarantined = append(output.Quarantined, QuarantinedPart{
					ArtifactID: artifact.ArtifactID, Filename: part.Filename, MediaType: part.MediaType,
					Kind: "raw", Reason: "inline binary parts require explicit quarantine review",
					SHA256: hex.EncodeToString(digest[:]),
				})
			case part.Text != nil:
				if len(*part.Text) > maxPartBytes {
					output.Quarantined = append(output.Quarantined, QuarantinedPart{
						ArtifactID: artifact.ArtifactID, Filename: part.Filename, MediaType: part.MediaType,
						Kind: "text", Reason: "text part exceeds configured limit",
					})
					continue
				}
				text := *part.Text
				safe.Parts = append(safe.Parts, SafePart{Text: &text, Filename: part.Filename, MediaType: part.MediaType})
			case len(part.Data) > 0:
				if len(part.Data) > maxPartBytes {
					output.Quarantined = append(output.Quarantined, QuarantinedPart{
						ArtifactID: artifact.ArtifactID, Filename: part.Filename, MediaType: part.MediaType,
						Kind: "data", Reason: "data part exceeds configured limit",
					})
					continue
				}
				safe.Parts = append(safe.Parts, SafePart{Data: append(json.RawMessage(nil), part.Data...), Filename: part.Filename, MediaType: part.MediaType})
			}
		}
		if len(safe.Parts) > 0 {
			output.Artifacts = append(output.Artifacts, safe)
		}
	}
	return output
}

func SanitizeMessage(message Message, maxPartBytes int) MessageOutput {
	synthetic := Task{
		ID: "message-" + message.MessageID,
		Status: TaskStatus{
			State: TaskStateCompleted,
		},
		Artifacts: []Artifact{{ArtifactID: "message", Parts: message.Parts}},
	}
	taskOutput := SanitizeTask(synthetic, maxPartBytes)
	output := MessageOutput{ProtocolVersion: ProtocolVersion, MessageID: message.MessageID, Quarantined: taskOutput.Quarantined}
	if len(taskOutput.Artifacts) > 0 {
		output.Parts = taskOutput.Artifacts[0].Parts
	}
	return output
}
