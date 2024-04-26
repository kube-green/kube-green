package statefulsets

import (
	"testing"

	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"

	"github.com/stretchr/testify/require"
)

func TestStatefulSetOriginalReplicas(t *testing.T) {
	namespace := "my-namespace"
	var replica1 int32 = 1
	var replica5 int32 = 5

	ss1 := mocks.StatefulSet(mocks.StatefulSetOptions{
		Namespace:       namespace,
		Name:            "ss1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	}).Resource()
	ss2 := mocks.StatefulSet(mocks.StatefulSetOptions{
		Namespace:       namespace,
		Name:            "ss2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	}).Resource()

	t.Run("restore saved info", func(t *testing.T) {
		expectedInfoToSave := `[{"name":"ss1","replicas":1},{"name":"ss2","replicas":5}]`
		infoToSave := []byte(expectedInfoToSave)
		restoredInfo, err := GetOriginalInfoToRestore(infoToSave)
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			ss1.Name: `{"spec":{"replicas":1}}`,
			ss2.Name: `{"spec":{"replicas":5}}`,
		}, restoredInfo)
	})

	t.Run("restore info with data nil", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore(nil)
		require.Equal(t, map[string]string{}, info)
		require.NoError(t, err)
	})

	t.Run("fails if saved data are not valid json", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore([]byte(`{}`))
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []statefulsets.OriginalReplicas")
		require.Nil(t, info)
	})
}
