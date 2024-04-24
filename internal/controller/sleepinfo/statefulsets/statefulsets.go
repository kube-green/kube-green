package statefulsets

import (
	"context"
	"encoding/json"

	kubegreenv1alpha1 "github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type statefulsets struct {
	resource.ResourceClient
	data             []appsv1.StatefulSet
	OriginalReplicas map[string]int32
	areToSuspend     bool
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalReplicas map[string]int32) (resource.Resource, error) {
	s := statefulsets{
		ResourceClient:   res,
		OriginalReplicas: originalReplicas,
		data:             []appsv1.StatefulSet{},
		areToSuspend:     res.SleepInfo.IsStatefulsetsToSuspend(),
	}
	if !s.areToSuspend {
		return s, nil
	}
	if err := s.fetch(ctx, namespace); err != nil {
		return statefulsets{}, err
	}

	return s, nil
}

func (s statefulsets) HasResource() bool {
	return len(s.data) > 0
}

func (s statefulsets) Sleep(ctx context.Context) error {
	for _, statefulset := range s.data {
		statefulset := statefulset

		statefulsetReplicas := *statefulset.Spec.Replicas
		if statefulsetReplicas == 0 {
			continue
		}

		newStatefulset := statefulset.DeepCopy()
		*newStatefulset.Spec.Replicas = 0

		if err := s.Patch(ctx, &statefulset, newStatefulset); err != nil {
			return err
		}
	}
	return nil
}

func (s statefulsets) WakeUp(ctx context.Context) error {
	for _, statefulset := range s.data {
		statefulset := statefulset

		statefulsetLogger := s.Log.WithValues("statefulset", statefulset.Name, "namespace", statefulset.Namespace)

		if *statefulset.Spec.Replicas != 0 {
			statefulsetLogger.Info("replicas not 0 during wake up")
			continue
		}

		replica, ok := s.OriginalReplicas[statefulset.Name]
		if !ok {
			statefulsetLogger.Info("original statefulset info not correctly set")
			continue
		}

		newStatefulset := statefulset.DeepCopy()
		*newStatefulset.Spec.Replicas = replica

		if err := s.Patch(ctx, &statefulset, newStatefulset); err != nil {
			return err
		}
	}
	return nil
}

func (s *statefulsets) fetch(ctx context.Context, namespace string) error {
	log := s.Log.WithValues("namespace", namespace)

	statefulsetList, err := s.getListByNamespace(ctx, namespace)
	if err != nil {
		return err
	}
	log.V(8).Info("statefulsets in namespace", "number of statefulsets", len(statefulsetList))
	s.data = s.filterExcludedStatefulset(statefulsetList)
	return nil
}

func (s statefulsets) getListByNamespace(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}
	statefulsets := appsv1.StatefulSetList{}
	if err := s.Client.List(ctx, &statefulsets, listOptions); err != nil {
		return statefulsets.Items, client.IgnoreNotFound(err)
	}
	return statefulsets.Items, nil
}

func (s statefulsets) filterExcludedStatefulset(statefulsetList []appsv1.StatefulSet) []appsv1.StatefulSet {
	filtered := []appsv1.StatefulSet{}
	for _, statefulset := range statefulsetList {
		if !shouldExcludeStatefulset(statefulset, s.SleepInfo) {
			filtered = append(filtered, statefulset)
		}
	}
	return filtered
}

func shouldExcludeStatefulset(statefulset appsv1.StatefulSet, sleepInfo *kubegreenv1alpha1.SleepInfo) bool {
	for _, exclusion := range sleepInfo.GetExcludeRef() {
		if exclusion.Kind == "StatefulSet" && exclusion.APIVersion == "apps/v1" && exclusion.Name != "" && statefulset.Name == exclusion.Name {
			return true
		}
		if labelMatch(statefulset.Labels, exclusion.MatchLabels) {
			return true
		}
	}
	return false
}

func labelMatch(labels, matchLabels map[string]string) bool {
	if len(matchLabels) == 0 {
		return false
	}

	matched := true
	for key, value := range matchLabels {
		v, ok := labels[key]
		if !ok || v != value {
			matched = false
			break
		}
	}

	return matched
}

type OriginalReplicas struct {
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

func (s statefulsets) GetOriginalInfoToSave() ([]byte, error) {
	if !s.areToSuspend {
		return nil, nil
	}
	originalStatefulsetsReplicas := []OriginalReplicas{}
	for _, statefulset := range s.data {
		originalReplicas := *statefulset.Spec.Replicas
		if replica, ok := s.OriginalReplicas[statefulset.Name]; ok {
			if ok && replica != 0 {
				originalReplicas = replica
			}
		}
		if originalReplicas == 0 {
			continue
		}
		originalStatefulsetsReplicas = append(originalStatefulsetsReplicas, OriginalReplicas{
			Name:     statefulset.Name,
			Replicas: originalReplicas,
		})
	}
	return json.Marshal(originalStatefulsetsReplicas)
}

func GetOriginalInfoToRestore(data []byte) (map[string]int32, error) {
	if data == nil {
		return map[string]int32{}, nil
	}
	originalStatefulsetsReplicas := []OriginalReplicas{}
	originalStatefulsetsReplicasData := map[string]int32{}
	if err := json.Unmarshal(data, &originalStatefulsetsReplicas); err != nil {
		return nil, err
	}
	for _, replicaInfo := range originalStatefulsetsReplicas {
		if replicaInfo.Name != "" {
			originalStatefulsetsReplicasData[replicaInfo.Name] = replicaInfo.Replicas
		}
	}
	return originalStatefulsetsReplicasData, nil
}
