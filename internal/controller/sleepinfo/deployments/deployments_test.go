package deployments

import (
	"testing"

	"github.com/kube-green/kube-green/internal/controller/sleepinfo/internal/mocks"

	"github.com/stretchr/testify/require"
)

func TestDeploymentOriginalReplicas(t *testing.T) {
	namespace := "my-namespace"
	var replica1 int32 = 1
	var replica5 int32 = 5

	d1 := mocks.Deployment(mocks.DeploymentOptions{
		Namespace:       namespace,
		Name:            "d1",
		Replicas:        &replica1,
		ResourceVersion: "2",
	}).Resource()
	d2 := mocks.Deployment(mocks.DeploymentOptions{
		Namespace:       namespace,
		Name:            "d2",
		Replicas:        &replica5,
		ResourceVersion: "1",
	}).Resource()

	t.Run("restore saved info", func(t *testing.T) {
		expectedInfoToSave := `[{"name":"d1","replicas":1},{"name":"d2","replicas":5}]`
		infoToSave := []byte(expectedInfoToSave)
		restoredInfo, err := GetOriginalInfoToRestore(infoToSave)
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			d1.Name: `{"spec":{"replicas":1}}`,
			d2.Name: `{"spec":{"replicas":5}}`,
		}, restoredInfo)
	})

	t.Run("restore info with data nil", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore(nil)
		require.Equal(t, map[string]int32{}, info)
		require.NoError(t, err)
	})

	t.Run("fails if saved data are not valid json", func(t *testing.T) {
		info, err := GetOriginalInfoToRestore([]byte(`{}`))
		require.EqualError(t, err, "json: cannot unmarshal object into Go value of type []deployments.OriginalReplicas")
		require.Nil(t, info)
	})
}
