package session

import (
	"claude-squad/config"
	"encoding/json"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Status    Status    `json:"status"`
	Height    int       `json:"height"`
	Width     int       `json:"width"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AutoYes   bool      `json:"auto_yes"`

	Program   string          `json:"program"`
	Worktree  GitWorktreeData `json:"worktree"`
	DiffStats DiffStatsData   `json:"diff_stats"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath         string `json:"repo_path"`
	WorktreePath     string `json:"worktree_path"`
	SessionName      string `json:"session_name"`
	BranchName       string `json:"branch_name"`
	BaseCommitSHA    string `json:"base_commit_sha"`
	BaseRef          string `json:"base_ref"`
	IsExistingBranch bool   `json:"is_existing_branch"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added        int    `json:"added"`
	Removed      int    `json:"removed"`
	Content      string `json:"content"`
	FilesChanged int    `json:"files_changed"`
	Commits      int    `json:"commits"`
	Behind       int    `json:"behind"`
	Dirty        bool   `json:"dirty"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// SaveInstances saves the list of instances to disk
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData
	data := make([]InstanceData, 0)
	for _, instance := range instances {
		if instance.Started() {
			data = append(data, instance.ToInstanceData())
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// loadInstanceData deserializes the persisted JSON into InstanceData records without
// reconstructing live Instance objects (no tmux/PTY sessions are opened).
func (s *Storage) loadInstanceData() ([]InstanceData, error) {
	jsonData := s.state.GetInstances()
	var data []InstanceData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}
	return data, nil
}

// saveInstanceData marshals a slice of InstanceData and persists it without
// constructing any live Instance objects.
func (s *Storage) saveInstanceData(data []InstanceData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}
	return s.state.SaveInstances(jsonData)
}

// LoadInstances loads the list of instances from disk
func (s *Storage) LoadInstances() ([]*Instance, error) {
	instancesData, err := s.loadInstanceData()
	if err != nil {
		return nil, err
	}

	instances := make([]*Instance, len(instancesData))
	for i, data := range instancesData {
		instance, err := FromInstanceData(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %w", data.Title, err)
		}
		instances[i] = instance
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage by title.
// It operates directly on the serialized InstanceData so it never opens PTY
// sessions for the surviving instances.
func (s *Storage) DeleteInstance(title string) error {
	all, err := s.loadInstanceData()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	keep := make([]InstanceData, 0, len(all))
	for _, d := range all {
		if d.Title != title {
			keep = append(keep, d)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.saveInstanceData(keep)
}

// UpdateInstance updates an existing instance in storage.
// It operates directly on the serialized InstanceData so it never opens PTY
// sessions for the other instances.
func (s *Storage) UpdateInstance(instance *Instance) error {
	all, err := s.loadInstanceData()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	updated := instance.ToInstanceData()
	found := false
	for i, d := range all {
		if d.Title == updated.Title {
			all[i] = updated
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", updated.Title)
	}

	return s.saveInstanceData(all)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}
