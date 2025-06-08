package httpfs

import (
	"encoding/json"
	"fmt"
	"time"
)

// Represents user input for node creation. It should be passed from entrypoints (i.e. cli, socket/web api, etc layers) to the filesystem Create methods
type NodeCreateRequest struct {
	NodeRequestor
	AttrCreateRequest
	Path string
	Type NodeCreateRequestType
	UUID *string `json:"uuid,omitempty"` // Optional UUID to enable linking at request time
}

type AttrCreateRequest struct {
	// If size not known ahead of time set to nil and the fs will get it from the
	// adapter
	Size     *uint64
	Atime    *time.Time // Last Accessed at (Default current time)
	Mtime    *time.Time // Last Modified at (Default current time)
	Ctime    *time.Time // Created at (Default current time)
	Perms    *uint32    // i.e. 0755
	OwnerUid *uint32
	OwnerGid *uint32
	// Blksize is the preferred size for file system operations.
	Blksize *uint32
}

// Implement NodeRequestor interface for the base type
func (r *NodeCreateRequest) GetType() NodeCreateRequestType {
	return r.Type
}

func (r *NodeCreateRequest) GetPath() string {
	return r.Path
}

func (r *NodeCreateRequest) GetAttr() *AttrCreateRequest {
	return &r.AttrCreateRequest
}

type NodeCreateRequestType string

const (
	FileNodeType     NodeCreateRequestType = "file"
	DirNodeType      NodeCreateRequestType = "dir"
	HardlinkNodeType NodeCreateRequestType = "hardlink"
	SymlinkNodeType  NodeCreateRequestType = "symlink"
)

type FileCreateRequest struct {
	NodeCreateRequest
	Sources []FileSourcer `json:"sources"`
}

type DirCreateRequest struct {
	NodeCreateRequest
}

// Implement NodeRequestor interface for DirCreateRequest
func (r *DirCreateRequest) GetNodeRequest() *NodeCreateRequest {
	return &r.NodeCreateRequest
}

type SourceType string

const (
	HttpSourceType SourceType = "http"
)

type SourceCreateRequest struct {
	FileSourcer
	Type     SourceType `json:"type"`
	Priority *int       `json:"priority,omitempty"` // Lower number = higher priority, defaults to array index
}

// Implement FileSourcer interface
func (r *SourceCreateRequest) GetSourceRequest() *SourceCreateRequest {
	return r
}

func (r *SourceCreateRequest) GetType() SourceType {
	return r.Type
}

// HTTP-specific source request
type HttpSourceCreateRequest struct {
	SourceCreateRequest
	Headers      map[string]string `json:"headers,omitempty"`
	Timeout      *int              `json:"timeout,omitempty"` // Timeout in seconds
	MaxRedirects *int              `json:"maxRedirects,omitempty"`
}

func (r *HttpSourceCreateRequest) GetSourceRequest() *SourceCreateRequest {
	return &r.SourceCreateRequest
}

func UnmarshalNodeRequest(data []byte) (NodeRequestor, error) {
	var nodeReq NodeCreateRequest
	if err := json.Unmarshal(data, &nodeReq); err != nil {
		return nil, err
	}

	// Compose core Node into the concrete struct
	switch nodeReq.Type {
	case FileNodeType:

		var rawSources struct {
			Sources []json.RawMessage `json:"sources"`
		}
		if err := json.Unmarshal(data, &rawSources); err != nil {
			return nil, fmt.Errorf("failed to extract sources data: %w", err)
		}

		var sources []FileSourcer
		for i, rawSource := range rawSources.Sources {
			sourceReq, err := UnmarshalSourceRequest(rawSource)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal source %d: %w", i, err)
			}

			sources = append(sources, sourceReq)
		}

		return &FileCreateRequest{
			NodeCreateRequest: nodeReq,
			Sources:           sources,
		}, nil
	case DirNodeType:
		var req DirCreateRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return &req, nil

	default:
		return nil, fmt.Errorf("unknown node type: %s", nodeReq.Type)
	}
}

func UnmarshalSourceRequest(rawSource json.RawMessage) (FileSourcer, error) {
	// Determine the source type
	var typeCheck struct {
		Type SourceType `json:"type"`
	}
	if err := json.Unmarshal(rawSource, &typeCheck); err != nil {
		return nil, fmt.Errorf("failed to determine source type: %w", err)
	}

	// Use the registry from factory.go instead of hardcoded switch
	return unmarshalRegisteredSource(typeCheck.Type, rawSource)
}
