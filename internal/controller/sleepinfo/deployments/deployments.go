package deployments

import (
	"encoding/json"
)

type OriginalReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func GetOriginalInfoToRestore(data []byte) (map[string]int32, error) {
	if data == nil {
		return map[string]int32{}, nil
	}
	originalDeploymentsReplicas := []OriginalReplicas{}
	originalDeploymentsReplicasData := map[string]int32{}
	if err := json.Unmarshal(data, &originalDeploymentsReplicas); err != nil {
		return nil, err
	}
	for _, replicaInfo := range originalDeploymentsReplicas {
		if replicaInfo.Name != "" {
			originalDeploymentsReplicasData[replicaInfo.Name] = replicaInfo.Replicas
		}
	}
	return originalDeploymentsReplicasData, nil
}
