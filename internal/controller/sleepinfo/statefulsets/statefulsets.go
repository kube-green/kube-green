package statefulsets

import (
	"encoding/json"
	"fmt"
)

type OriginalReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func GetOriginalInfoToRestore(data []byte) (map[string]string, error) {
	if data == nil {
		return map[string]string{}, nil
	}
	originalStatefulSetsReplicas := []OriginalReplicas{}
	originalStatefulSetsReplicasData := map[string]string{}
	if err := json.Unmarshal(data, &originalStatefulSetsReplicas); err != nil {
		return nil, err
	}
	for _, replicaInfo := range originalStatefulSetsReplicas {
		if replicaInfo.Name != "" {
			originalStatefulSetsReplicasData[replicaInfo.Name] = fmt.Sprintf("{\"spec\":{\"replicas\":%d}}", replicaInfo.Replicas)
		}
	}
	return originalStatefulSetsReplicasData, nil
}
