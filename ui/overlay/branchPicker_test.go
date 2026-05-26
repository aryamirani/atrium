package overlay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Changing the target repo invalidates the branch results, but the focused picker must
// keep its height and show a "searching…" hint rather than blanking to "No matching
// branches" — otherwise the list flickers and the overlay jumps on every directory move.
func TestBranchPicker_RenderHeightConstantWhileLoading(t *testing.T) {
	bp := NewBranchPicker()
	bp.Focus()
	bp.SetResults([]string{"main", "develop", "feature"}, bp.GetFilterVersion())
	withResults := strings.Count(bp.Render(), "\n")

	bp.Invalidate() // directory changed: results cleared, now loading
	out := bp.Render()

	assert.Equal(t, withResults, strings.Count(out, "\n"), "height must not change while reloading")
	assert.Contains(t, out, "searching")
	assert.NotContains(t, out, "No matching branches")
}

// SetResults with a matching version clears the loading state.
func TestBranchPicker_SetResultsClearsLoading(t *testing.T) {
	bp := NewBranchPicker()
	bp.Focus()
	version := bp.Invalidate()
	assert.Contains(t, bp.Render(), "searching")

	bp.SetResults([]string{"main"}, version)
	assert.NotContains(t, bp.Render(), "searching")
}
