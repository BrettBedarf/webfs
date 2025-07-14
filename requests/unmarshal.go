package requests

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/adapters"
)

// GetNodeType extracts the node type from JSON without full unmarshaling
func GetNodeType(data []byte) (webfs.NodeCreateRequestType, error) {
	var meta struct {
		Type webfs.NodeCreateRequestType `json:"type"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	return meta.Type, nil
}

// UnmarshalFileRequest handles file-specific unmarshaling with sources
func UnmarshalFileRequest(data []byte) (*webfs.FileCreateRequest, error) {
	var dto FileRequestDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}

	// Convert DTO to core type with defaults applied
	coreNode := convertNodeDTO(dto.NodeRequestDTO)

	sources, err := unmarshalSources(dto.Sources, data)
	if err != nil {
		return nil, err
	}

	return &webfs.FileCreateRequest{
		NodeRequest: coreNode,
		Sources:     sources,
	}, nil
}

// UnmarshalDirRequest handles explicit directory unmarshaling (no sources)
func UnmarshalDirRequest(data []byte) (*webfs.DirCreateRequest, error) {
	var dto DirRequestDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}

	return &webfs.DirCreateRequest{
		NodeRequest: convertNodeDTO(dto.NodeRequestDTO),
	}, nil
}

// Helper function to process sources array
func unmarshalSources(sourceDTOs []SourceConfigDTO, rawData []byte) ([]webfs.FileSource, error) {
	// Extract raw sources array from JSON for adapter registry
	var rawMessage struct {
		Sources []json.RawMessage `json:"sources"`
	}
	json.Unmarshal(rawData, &rawMessage)

	var sources []webfs.FileSource
	for i, rawSource := range rawMessage.Sources {
		// Use adapter registry to get provider
		provider, err := adapters.GetProvider(rawSource)
		if err != nil {
			return nil, err
		}

		// Apply priority default
		priority := i
		if sourceDTOs[i].Priority != nil {
			priority = *sourceDTOs[i].Priority
		}

		sources = append(sources, webfs.FileSource{
			AdapterProvider: provider,
			Priority:        priority,
		})
	}

	return sources, nil
}

// Conversion logic with defaults in the unmarshaling layer
// TODO: Use global configuration defaults
func convertNodeDTO(dto NodeRequestDTO) webfs.NodeRequest {
	now := time.Now()

	return webfs.NodeRequest{
		Path:     dto.Path,
		Type:     dto.Type,
		UUID:     valueOrDefault(dto.UUID, uuid.New().String()),
		Size:     valueOrDefault(dto.Size, 0),
		Atime:    valueOrDefault(dto.Atime, now),
		Mtime:    valueOrDefault(dto.Mtime, now),
		Ctime:    valueOrDefault(dto.Ctime, now),
		Perms:    valueOrDefault(dto.Perms, 0o644),
		OwnerUID: valueOrDefault(dto.OwnerUID, 1000),
		OwnerGID: valueOrDefault(dto.OwnerGID, 1000),
		Blksize:  valueOrDefault(dto.Blksize, 4096),
	}
}

func valueOrDefault[T any](ptr *T, defaultVal T) T {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}
