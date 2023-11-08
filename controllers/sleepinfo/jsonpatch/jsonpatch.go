package jsonpatch

import (
	"context"
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/controllers/sleepinfo/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type genericResource struct {
	resource.ResourceClient
	OriginalData   any
	data           []unstructured.Unstructured
	patchData      *v1alpha1.PatchJson6902
	restorePatches map[string][]byte
}

func NewResource(ctx context.Context, res resource.ResourceClient, namespace string, originalData map[string][]map[string]any) (resource.Resource, error) {
	patchData := &res.SleepInfo.GetPatchesJson6902()[0] // TODO: support multiple patches or pass patches as param
	generic := genericResource{
		ResourceClient: res,
		OriginalData:   originalData,
		data:           []unstructured.Unstructured{},
		patchData:      patchData,
		restorePatches: map[string][]byte{},
	}

	resourceList, err := generic.getListByNamespace(ctx, namespace, patchData)
	if err != nil {
		return nil, err
	}
	generic.data = resourceList

	return generic, nil
}

func (g genericResource) HasResource() bool {
	return len(g.data) > 0
}

func (g genericResource) Sleep(ctx context.Context) error {
	patch, err := jsonpatch.DecodePatch([]byte(g.patchData.Patches))
	if err != nil {
		return err
	}

	for _, resource := range g.data {
		// TODO: test this
		// remove resourceVersion from patch target for SSA patch to work correctly
		unstructured.RemoveNestedField(resource.Object, "metadata", "resourceVersion")

		original, err := json.Marshal(resource.Object)
		if err != nil {
			return err
		}

		modified, err := patch.Apply(original)
		if err != nil {
			g.Log.Error(err, "fails to apply patch",
				"resourceName", resource.GetName(),
				"resourceKind", resource.GetKind(),
				"patch", g.patchData.Patches,
			)
			continue
		}

		restorePatch, err := jsonpatch.CreateMergePatch(modified, original)
		if err != nil {
			return err
		}
		g.restorePatches[resource.GetName()] = restorePatch

		res := &unstructured.Unstructured{}
		if err := json.Unmarshal(modified, &res.Object); err != nil {
			return err
		}

		if err := g.SSAPatch(ctx, res); err != nil {
			return err
		}
	}

	return nil
}

func (g genericResource) WakeUp(ctx context.Context) error {
	for _, resource := range g.data {
		rawPatch, ok := g.restorePatches[resource.GetName()]
		if !ok {
			// TODO: log
			g.Log.Info("no restore patch found for resource, skipped",
				"resourceName", resource.GetName(),
				"resourceKind", resource.GetKind(),
			)
			continue
		}
		// TODO: test this
		// remove resourceVersion from patch target for SSA patch to work correctly
		unstructured.RemoveNestedField(resource.Object, "metadata", "resourceVersion")

		original, err := json.Marshal(resource.Object)
		if err != nil {
			return err
		}

		restored, err := jsonpatch.MergePatch(original, rawPatch)
		if err != nil {
			return err
		}

		res := &unstructured.Unstructured{}
		if err := json.Unmarshal(restored, &res.Object); err != nil {
			return err
		}

		if err := g.SSAPatch(ctx, res); err != nil {
			return err
		}
	}

	return nil
}

func (g genericResource) GetOriginalInfoToSave() ([]byte, error) {
	return nil, nil
}

func (c genericResource) getListByNamespace(ctx context.Context, namespace string, patchData *v1alpha1.PatchJson6902) ([]unstructured.Unstructured, error) {
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
		Group: patchData.Target.Group,
		Kind:  patchData.Target.Kind,
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
