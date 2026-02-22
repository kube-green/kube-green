package jsonpatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/patcher"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrJSONPatch     = fmt.Errorf("jsonpatch error")
	ErrListResources = fmt.Errorf("list resources error")
)

type managedResources struct {
	logger     logr.Logger
	resMapping map[v1alpha1.PatchTarget]*genericResource
	namespace  string
}

type RestorePatches map[string]string

func NewResources(ctx context.Context, res resource.ResourceClient, namespace string, restorePatches map[string]RestorePatches) (resource.Resource, error) {
	if res.SleepInfo == nil {
		return nil, fmt.Errorf("%w: sleepInfo is not provided", ErrJSONPatch)
	}
	resources := managedResources{
		logger:     res.Log,
		resMapping: map[v1alpha1.PatchTarget]*genericResource{},
		namespace:  namespace,
	}
	if restorePatches == nil {
		restorePatches = map[string]RestorePatches{}
	}

	for _, patchData := range res.SleepInfo.GetPatches() {
		res.Log.V(8).Info("patch data", "patch", patchData.Patch, "target", patchData.Target)
		restorePatch, ok := restorePatches[patchData.Target.String()]
		if !ok {
			restorePatch = RestorePatches{}
		}

		generic := newGenericResource(res, patchData, restorePatch)

		var err error
		generic.data, err = generic.getListByNamespace(ctx, namespace, patchData.Target)
		if err != nil {
			res.Log.WithValues("target", patchData.Target.String()).Error(err, "fails to get list of resources")
			continue
		}
		if len(generic.data) == 0 {
			continue
		}

		resources.resMapping[patchData.Target] = generic
	}

	return resources, nil
}

func (g managedResources) HasResource() bool {
	for _, res := range g.resMapping {
		if len(res.data) > 0 {
			return true
		}
	}
	return false
}

func (g managedResources) Sleep(ctx context.Context) error {
	for _, resourceWrapper := range g.resMapping {
		if resourceWrapper.patchData.Patch == "" {
			return fmt.Errorf(`%w: invalid empty patch`, ErrJSONPatch)
		}

		patcherFn, err := patcher.New([]byte(resourceWrapper.patchData.Patch))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		if resourceWrapper.isCacheInvalid {
			resourceWrapper.data, err = resourceWrapper.getListByNamespace(ctx, g.namespace, resourceWrapper.patchData.Target)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrListResources, err)
			}
		}

		for _, resource := range resourceWrapper.data {
			// This will skip resources that are managed by another controller, since
			// we should manage the sleep on the controller itself.
			// Some examples are:
			// - Pod managed by ReplicaSet managed by Deployment
			// - Pod managed by Job managed by CronJob
			if metav1.GetControllerOfNoCopy(&resource) != nil {
				g.logger.Info("resource is managed by another controller, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}

			original, err := json.Marshal(resource.Object)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			modified, err := patcherFn.Exec(original)
			if err != nil {
				g.logger.Error(err, "fails to apply patch",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}

			restorePatch, err := jsonpatch.CreateMergePatch(modified, original)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}
			restorePatchString := string(restorePatch)

			// an empty patch means that the resource is not changed, so we can skip it
			isEmptyPatch := restorePatchString == "{}"
			if isEmptyPatch {
				continue
			}

			resourceWrapper.restorePatches[resource.GetName()] = restorePatchString

			// Create forward merge patch containing only the changed fields
			forwardPatch, err := jsonpatch.CreateMergePatch(original, modified)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			// Build sparse object with identifiers + changed fields only
			// This ensures SSA only claims ownership of fields we actually modify
			sparseObj, err := buildSparseApplyObject(resource, forwardPatch)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			if err := resourceWrapper.SSAPatch(ctx, sparseObj); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}
			resourceWrapper.isCacheInvalid = true
		}
	}

	return nil
}

func (g managedResources) WakeUp(ctx context.Context) error {
	for _, resourceWrapper := range g.resMapping {
		if resourceWrapper.isCacheInvalid {
			var err error
			resourceWrapper.data, err = resourceWrapper.getListByNamespace(ctx, g.namespace, resourceWrapper.patchData.Target)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrListResources, err)
			}
		}

		patcherFn, err := patcher.New([]byte(resourceWrapper.patchData.Patch))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		for _, resource := range resourceWrapper.data {
			rawPatch, ok := resourceWrapper.restorePatches[resource.GetName()]
			if !ok {
				g.logger.Info("no restore patch found for resource, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
				)
				continue
			}

			current, err := json.Marshal(resource.Object)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			isResourceChanged, err := patcherFn.IsResourceChanged(current)
			if err != nil {
				g.logger.Error(err, "fails to calculate if resource is changed",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}
			if isResourceChanged {
				g.logger.Info("resource modified between sleep and wake up, skip wake up",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}

			restored, err := jsonpatch.MergePatch(current, []byte(rawPatch))
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			res := &unstructured.Unstructured{}
			if err := json.Unmarshal(restored, &res.Object); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			// Here we need to use Patch because SSA patch will not work for restore,
			// for example if we should remove an object, SSA patch will not work correctly
			// (the applied resources does not have the object removed, so SSA patch will not remove it.
			// To work properly, the value of the object should be null)
			if err := resourceWrapper.Patch(ctx, resource.DeepCopy(), res); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}
			resourceWrapper.isCacheInvalid = true
		}
	}

	return nil
}

func (g managedResources) GetOriginalInfoToSave() ([]byte, error) {
	if len(g.resMapping) == 0 {
		return nil, nil
	}

	dataToSave := map[string]RestorePatches{}
	for key, res := range g.resMapping {
		if len(res.restorePatches) == 0 {
			continue
		}
		dataToSave[key.String()] = res.restorePatches
	}

	return json.Marshal(dataToSave)
}

func GetOriginalInfoToRestore(data []byte) (map[string]RestorePatches, error) {
	if data == nil {
		return nil, nil
	}

	resourcePatches := map[string]RestorePatches{}
	if err := json.Unmarshal(data, &resourcePatches); err != nil {
		return nil, err
	}

	return resourcePatches, nil
}

// buildSparseApplyObject creates a sparse unstructured object containing only:
// - Required identifiers (apiVersion, kind, metadata.name, metadata.namespace)
// - The fields that were actually changed (from the merge patch)
// This ensures that SSA only claims ownership of the fields we modify.
func buildSparseApplyObject(resource unstructured.Unstructured, mergePatch []byte) (*unstructured.Unstructured, error) {
	var patchMap map[string]any
	if err := json.Unmarshal(mergePatch, &patchMap); err != nil {
		return nil, err
	}

	sparse := &unstructured.Unstructured{Object: map[string]any{}}

	for key, value := range patchMap {
		err := unstructured.SetNestedField(sparse.Object, value, key)
		if err != nil {
			return nil, err
		}
	}

	sparse.SetGroupVersionKind(resource.GroupVersionKind())
	sparse.SetName(resource.GetName())
	sparse.SetNamespace(resource.GetNamespace())

	return sparse, nil
}
