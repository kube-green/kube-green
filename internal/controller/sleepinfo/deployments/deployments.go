package deployments

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
	originalDeploymentsReplicas := []OriginalReplicas{}
	originalDeploymentsReplicasData := map[string]string{}
	if err := json.Unmarshal(data, &originalDeploymentsReplicas); err != nil {
		return nil, err
	}
	for _, replicaInfo := range originalDeploymentsReplicas {
		if replicaInfo.Name != "" {
			originalDeploymentsReplicasData[replicaInfo.Name] = fmt.Sprintf("{\"spec\":{\"replicas\":%d}}", replicaInfo.Replicas)
		}
	}
	return originalDeploymentsReplicasData, nil
}
