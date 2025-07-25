package cronjobs

import (
	"encoding/json"
	"fmt"
)

type OriginalCronJobStatus struct {
	Name    string `json:"name"`
	Suspend bool   `json:"suspend"`
}

func GetOriginalInfoToRestore(savedData []byte) (map[string]string, error) {
	if savedData == nil {
		return map[string]string{}, nil
	}
	originalSuspendedCronJob := []OriginalCronJobStatus{}
	if err := json.Unmarshal(savedData, &originalSuspendedCronJob); err != nil {
		return nil, err
	}
	originalSuspendedCronjobData := map[string]string{}
	for _, cronJob := range originalSuspendedCronJob {
		if cronJob.Name != "" {
			originalSuspendedCronjobData[cronJob.Name] = fmt.Sprintf("{\"spec\":{\"suspend\":%t}}", cronJob.Suspend)
		}
	}
	return originalSuspendedCronjobData, nil
}
