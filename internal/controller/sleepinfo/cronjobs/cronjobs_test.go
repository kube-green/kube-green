package cronjobs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCronJobs(t *testing.T) {
	t.Run("GetOriginalInfoToRestore", func(t *testing.T) {
		t.Run("if empty saved data, returns empty status", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore(nil)
			require.NoError(t, err)
			require.Equal(t, map[string]string{}, suspendedStatus)
		})

		t.Run("throws if data is not a correct json", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore([]byte("{"))
			require.Nil(t, suspendedStatus)
			require.EqualError(t, err, "unexpected end of JSON input")
		})

		t.Run("with empty data returns empty status", func(t *testing.T) {
			suspendedStatus, err := GetOriginalInfoToRestore([]byte("[]"))
			require.NoError(t, err)
			require.Equal(t, map[string]string{}, suspendedStatus)
		})

		t.Run("correctly returns data", func(t *testing.T) {
			savedData := []byte(`[{"name":"cj1","suspend":false},{"name":"cj2","suspend":true},{"name":"cj3","suspend":false},{"name":"","suspend":false}]`)
			suspendedStatus, err := GetOriginalInfoToRestore(savedData)
			require.NoError(t, err)
			require.Equal(t, map[string]string{
				"cj1": `{"spec":{"suspend":false}}`,
				"cj2": `{"spec":{"suspend":true}}`,
				"cj3": `{"spec":{"suspend":false}}`,
			}, suspendedStatus)
		})
	})
}
