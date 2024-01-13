package jsonpatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrJSONPatch     = fmt.Errorf("jsonpatch error")
	ErrListResources = fmt.Errorf("list resources error")
)

type managedResources struct {
	logger     logr.Logger
	resMapping map[string]*genericResource
	namespace  string
}

type RestorePatches map[string]string

func getTargetKey(target v1alpha1.PatchTarget) string {
	return fmt.Sprintf("%s-%s", target.Group, target.Kind)
}

// TODO: give some check on ownerReferences. Maybe does not change if kind already managed by kube-green?
func NewResources(ctx context.Context, res resource.ResourceClient, namespace string, restorePatches map[string]RestorePatches) (resource.Resource, error) {
	if res.SleepInfo == nil {
		return nil, fmt.Errorf("%w: sleepInfo is not provided", ErrJSONPatch)
	}
	resources := managedResources{
		logger: res.Log,
		// TODO: Use map[v1alpha1.PatchTarget]string
		resMapping: map[string]*genericResource{},
		namespace:  namespace,
	}
	if restorePatches == nil {
		restorePatches = map[string]RestorePatches{}
	}

	for _, patchData := range res.SleepInfo.GetPatches() {
		res.Log.V(8).Info("patch data", "patch", patchData.Patch, "target", patchData.Target)
		restorePatch, ok := restorePatches[getTargetKey(patchData.Target)]
		if !ok {
			restorePatch = RestorePatches{}
		}

		generic := newGenericResource(res, patchData, restorePatch)

		var err error
		generic.data, err = generic.getListByNamespace(ctx, namespace, patchData.Target)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrListResources, err)
		}

		// TODO: avoid to save if no resource is found
		resources.resMapping[getTargetKey(patchData.Target)] = generic
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

		patcherFn, err := CreatePatch([]byte(resourceWrapper.patchData.Patch))
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
			// TODO: test this
			// remove resourceVersion from patch target for SSA patch to work correctly
			unstructured.RemoveNestedField(resource.Object, "metadata", "resourceVersion")

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

			res := &unstructured.Unstructured{}
			if err := json.Unmarshal(modified, &res.Object); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			if err := resourceWrapper.SSAPatch(ctx, res); err != nil {
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

		patcherFn, err := CreatePatch([]byte(resourceWrapper.patchData.Patch))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		for _, resource := range resourceWrapper.data {
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

			rawPatch, ok := resourceWrapper.restorePatches[resource.GetName()]
			if !ok {
				g.logger.Info("no restore patch found for resource, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
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
		dataToSave[key] = res.restorePatches
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