package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inMemoryStorage is a minimal in-memory config.InstanceStorage for unit tests.
type inMemoryStorage struct {
	data json.RawMessage
}

func (s *inMemoryStorage) SaveInstances(b json.RawMessage) error {
	s.data = append([]byte(nil), b...)
	return nil
}
func (s *inMemoryStorage) GetInstances() json.RawMessage {
	if s.data == nil {
		return []byte("[]")
	}
	return s.data
}
func (s *inMemoryStorage) DeleteAllInstances() error {
	s.data = []byte("[]")
	return nil
}

// newPausedInstance is a test helper that creates an Instance in Paused state without
// starting tmux or git — safe for storage-layer tests because FromInstanceData never
// opens a PTY for paused instances.
func newPausedInstance(t *testing.T, title string) *Instance {
	t.Helper()
	inst, err := NewInstance(InstanceOptions{Title: title, Path: ".", Program: "echo"})
	require.NoError(t, err)
	inst.Status = Paused
	inst.started = true // mark started so ToInstanceData / SaveInstances includes it
	return inst
}

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	store, err := NewStorage(&inMemoryStorage{})
	require.NoError(t, err)
	return store
}

// TestStorageRoundTrip saves two paused instances and loads them back, asserting the
// in-memory store faithfully serialises and deserialises InstanceData.
func TestStorageRoundTrip(t *testing.T) {
	store := newTestStorage(t)

	a := newPausedInstance(t, "alpha")
	b := newPausedInstance(t, "beta")
	require.NoError(t, store.SaveInstances([]*Instance{a, b}))

	got, err := store.LoadInstances()
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "alpha", got[0].Title)
	assert.Equal(t, "beta", got[1].Title)
	assert.Equal(t, Paused, got[0].Status)
}

// TestDeleteInstance_RemovesNamedInstance confirms that DeleteInstance removes exactly
// the named entry and leaves the rest intact.
func TestDeleteInstance_RemovesNamedInstance(t *testing.T) {
	store := newTestStorage(t)
	a := newPausedInstance(t, "alpha")
	b := newPausedInstance(t, "beta")
	c := newPausedInstance(t, "gamma")
	require.NoError(t, store.SaveInstances([]*Instance{a, b, c}))

	require.NoError(t, store.DeleteInstance("beta"))

	remaining, err := store.LoadInstances()
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	titles := []string{remaining[0].Title, remaining[1].Title}
	assert.Contains(t, titles, "alpha")
	assert.Contains(t, titles, "gamma")
	assert.NotContains(t, titles, "beta")
}

// TestDeleteInstance_NotFoundReturnsError asserts that deleting a non-existent title
// returns an error rather than silently succeeding.
func TestDeleteInstance_NotFoundReturnsError(t *testing.T) {
	store := newTestStorage(t)
	require.NoError(t, store.SaveInstances([]*Instance{newPausedInstance(t, "alpha")}))

	err := store.DeleteInstance("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestUpdateInstance_UpdatesField confirms that UpdateInstance persists a changed
// displayName and leaves other instances untouched.
func TestUpdateInstance_UpdatesField(t *testing.T) {
	store := newTestStorage(t)
	a := newPausedInstance(t, "alpha")
	b := newPausedInstance(t, "beta")
	require.NoError(t, store.SaveInstances([]*Instance{a, b}))

	// Mutate alpha's display name and persist it.
	a.SetDisplayName("Alpha New Label")
	require.NoError(t, store.UpdateInstance(a))

	got, err := store.LoadInstances()
	require.NoError(t, err)
	require.Len(t, got, 2)

	var updatedAlpha, unchangedBeta *Instance
	for _, inst := range got {
		if inst.Title == "alpha" {
			updatedAlpha = inst
		} else if inst.Title == "beta" {
			unchangedBeta = inst
		}
	}
	require.NotNil(t, updatedAlpha)
	require.NotNil(t, unchangedBeta)
	assert.Equal(t, "Alpha New Label", updatedAlpha.DisplayName())
	assert.Equal(t, "beta", unchangedBeta.Title)
}

// TestUpdateInstance_NotFoundReturnsError asserts that updating a non-existent instance
// returns an error rather than silently appending a new entry.
func TestUpdateInstance_NotFoundReturnsError(t *testing.T) {
	store := newTestStorage(t)
	require.NoError(t, store.SaveInstances([]*Instance{newPausedInstance(t, "alpha")}))

	ghost := newPausedInstance(t, "ghost")
	err := store.UpdateInstance(ghost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestDeleteAllInstances_ClearsEverything confirms that DeleteAllInstances wipes all
// stored instances so a subsequent load returns an empty slice.
func TestDeleteAllInstances_ClearsEverything(t *testing.T) {
	store := newTestStorage(t)
	require.NoError(t, store.SaveInstances([]*Instance{newPausedInstance(t, "alpha"), newPausedInstance(t, "beta")}))

	require.NoError(t, store.DeleteAllInstances())

	got, err := store.LoadInstances()
	require.NoError(t, err)
	assert.Empty(t, got)
}
