package patcher

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("create new patcher", func(t *testing.T) {
		patcher, err := New([]byte(`
- op: add
  path: /some/path
  value: some-value
`))
		require.NoError(t, err)

		t.Run("exec patch", func(t *testing.T) {
			new, err := patcher.Exec([]byte(`{}`))
			require.NoError(t, err)
			require.JSONEq(t, `{"some":{"path":"some-value"}}`, string(new))
		})

		t.Run("is resource unchanged", func(t *testing.T) {
			isChanged, err := patcher.IsResourceChanged([]byte(`{"some":{"path":"some-value"}}`))
			require.NoError(t, err)
			require.False(t, isChanged)
		})

		t.Run("is resource changed", func(t *testing.T) {
			isChanged, err := patcher.IsResourceChanged([]byte(`{"some":{"path":"another-value"}}`))
			require.NoError(t, err)
			require.True(t, isChanged)
		})
	})

	t.Run("fails to create new patcher", func(t *testing.T) {
		_, err := New([]byte(`
- op: invalid
  path: /some/path
  value: some-value
`))
		require.EqualError(t, err, "invalid operation {\"op\":\"invalid\",\"path\":\"/some/path\",\"value\":\"some-value\"}: unsupported operation")
	})

	t.Run("fails if not correct yaml", func(t *testing.T) {
		_, err := New([]byte(`
	name: John Doe
`))
		require.ErrorContains(t, err, "yaml: ")
	})

	t.Run("fails to returns if resource are changed", func(t *testing.T) {
		patcher, err := New([]byte(`
- op: add
  path: /some/path
  value: some-value
`))
		require.NoError(t, err)

		_, err = patcher.IsResourceChanged([]byte(`not valid json`))
		require.EqualError(t, err, "invalid state detected")
	})
}
