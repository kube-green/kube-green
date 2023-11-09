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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrJSONPatch     = fmt.Errorf("jsonpatch error")
	ErrListResources = fmt.Errorf("list resources error")
)

type managedResources struct {
	logger     logr.Logger
	resMapping map[string]genericResource
	namespace  string
}

type genericResource struct {
	resource.ResourceClient
	patchData      v1alpha1.PatchJson6902
	restorePatches map[string][]byte
}

type ResourceList []map[string][]byte

func NewResources(ctx context.Context, res resource.ResourceClient, namespace string) (resource.Resource, error) {
	if res.SleepInfo == nil {
		return nil, fmt.Errorf("%w: sleepInfo is not provided", ErrJSONPatch)
	}
	resources := managedResources{
		logger:     res.Log,
		resMapping: map[string]genericResource{},
		namespace:  namespace,
	}

	for _, patchData := range res.SleepInfo.GetPatchesJson6902() {
		generic := genericResource{
			ResourceClient: res,
			patchData:      patchData,
			restorePatches: map[string][]byte{},
		}

		resources.resMapping[patchData.Target.Kind] = generic
	}

	return resources, nil
}

func (g managedResources) HasResource() bool {
	for _, res := range g.resMapping {
		if len(res.restorePatches) > 0 {
			return true
		}
	}
	return false
}

func (g managedResources) Sleep(ctx context.Context) error {
	for _, resourceWrapper := range g.resMapping {
		if resourceWrapper.patchData.Patches == "" {
			return fmt.Errorf(`%w: invalid empty patch`, ErrJSONPatch)
		}

		patcherFn, err := createPatch([]byte(resourceWrapper.patchData.Patches))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		resourceList, err := resourceWrapper.getListByNamespace(ctx, g.namespace, resourceWrapper.patchData.Target)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrListResources, err)
		}

		for _, resource := range resourceList {
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
					"patch", resourceWrapper.patchData.Patches,
				)
				continue
			}

			restorePatch, err := jsonpatch.CreateMergePatch(modified, original)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}
			resourceWrapper.restorePatches[resource.GetName()] = restorePatch

			res := &unstructured.Unstructured{}
			if err := json.Unmarshal(modified, &res.Object); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			if err := resourceWrapper.SSAPatch(ctx, res); err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}
		}
	}

	return nil
}

func (g managedResources) WakeUp(ctx context.Context) error {
	for _, resourceWrapper := range g.resMapping {
		resourceList, err := resourceWrapper.getListByNamespace(ctx, g.namespace, resourceWrapper.patchData.Target)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrListResources, err)
		}

		patcherFn, err := createPatch([]byte(resourceWrapper.patchData.Patches))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		for _, resource := range resourceList {
			current, err := json.Marshal(resource.Object)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			modified, err := patcherFn.Exec(current)
			if err != nil {
				g.logger.Error(err, "fails to apply patch",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patches,
				)
				continue
			}

			if !jsonpatch.Equal(current, modified) {
				// This means that the resource is modified
				g.logger.Info("resource modified between sleep and wake up, skip wake up",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patches,
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

			restored, err := jsonpatch.MergePatch(current, rawPatch)
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
		}
	}

	return nil
}

func (g managedResources) GetOriginalInfoToSave() ([]byte, error) {
	return nil, nil
}

func (c genericResource) getListByNamespace(ctx context.Context, namespace string, patchTarget v1alpha1.PatchTarget) ([]unstructured.Unstructured, error) {
	listOptions := &client.ListOptions{
		Namespace: namespace,
		Limit:     500,
	}

	// TODO: implement excludeRef [and include by labels?]
	// excludeRef := c.ResourceClient.SleepInfo.GetExcludeRef()
	// cronJobsToExclude := getCronJobNameToExclude(excludeRef)
	// cronJobLabelsToExclude := getCronJobLabelsToExclude(excludeRef)
	// fieldsSelector := []string{}
	// for _, cronJobToExclude := range cronJobsToExclude {
	// 	fieldsSelector = append(fieldsSelector, fmt.Sprintf("metadata.name!=%s", cronJobToExclude))
	// }
	// if len(fieldsSelector) > 0 {
	// 	fSel, err := fields.ParseSelector(strings.Join(fieldsSelector, ","))
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	listOptions.FieldSelector = fSel
	// }

	// if cronJobLabelsToExclude != nil {
	// 	labelSelector, err := labels.Parse(strings.Join(cronJobLabelsToExclude, ","))
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	listOptions.LabelSelector = labelSelector
	// }

	restMapping, err := c.Client.RESTMapper().RESTMapping(schema.GroupKind{
		Group: patchTarget.Group,
		Kind:  patchTarget.Kind,
	})
	if err != nil {
		return nil, err
	}

	resourceList := unstructured.UnstructuredList{}
	resourceList.SetGroupVersionKind(restMapping.GroupVersionKind)

	if err := c.Client.List(ctx, &resourceList, listOptions); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	return resourceList.Items, nil
}

type patcher struct {
	jsonpatch.Patch
}

func (p patcher) Exec(original []byte) ([]byte, error) {
	return p.ApplyWithOptions(original, &jsonpatch.ApplyOptions{
		EnsurePathExistsOnAdd:    true,
		AllowMissingPathOnRemove: true,
	})
}

func createPatch(patchToApply []byte) (*patcher, error) {
	patch, err := jsonpatch.DecodePatch(patchToApply)
	if err != nil {
		return nil, err
	}

	return &patcher{
		Patch: patch,
	}, nil
}
