package jsonpatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kube-green/kube-green/api/v1alpha1"
	"github.com/kube-green/kube-green/internal/controller/sleepinfo/resource"
	"github.com/kube-green/kube-green/internal/patcher"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			// EXTENSIÓN: Log cuando no se encuentran recursos para un patch target (útil para debugging CRDs)
			res.Log.Info("no resources found for patch target", "target", patchData.Target.String(), "namespace", namespace)
			continue
		}
		// EXTENSIÓN: Log cuando se encuentran recursos (útil para debugging CRDs)
		res.Log.Info("resources found for patch target", "target", patchData.Target.String(), "count", len(generic.data), "namespace", namespace)

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
			// EXCEPCIÓN: No saltar CRDs (PgBouncer, PgCluster, HDFSCluster) aunque tengan ownerReferences,
			// ya que estos son recursos de nivel superior que debemos gestionar directamente.
			resourceKind := resource.GetKind()
			isCRD := resourceKind == "PgBouncer" || resourceKind == "PgCluster" || resourceKind == "HDFSCluster"

			if metav1.GetControllerOfNoCopy(&resource) != nil && !isCRD {
				g.logger.Info("resource is managed by another controller, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resourceKind,
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}

			// CRITICAL: Save original state BEFORE attempting patch
			// This ensures we always have the original state saved, even if patch fails
			original, err := json.Marshal(resource.Object)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			// Now attempt to apply the patch
			modified, err := patcherFn.Exec(original)
			if err != nil {
				g.logger.Error(err, "fails to apply patch",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
					"patch", resourceWrapper.patchData.Patch,
				)
				// CRITICAL: Even if patch fails, we need to save the original state
				// The resource might have been modified in a previous attempt (e.g., replicas set to 0)
				// We need to re-read the current state from cluster and create a restore patch
				// that restores from current state to original state
				currentResource := &unstructured.Unstructured{}
				currentResource.SetGroupVersionKind(resource.GroupVersionKind())
				currentResource.SetName(resource.GetName())
				currentResource.SetNamespace(resource.GetNamespace())
				
				// Re-read current state from cluster (might be modified from previous attempt)
				if err := resourceWrapper.Client.Get(ctx, client.ObjectKeyFromObject(currentResource), currentResource); err != nil {
					g.logger.Error(err, "failed to re-read resource after patch failure, using original state",
						"resourceName", resource.GetName(),
						"resourceKind", resource.GetKind(),
					)
					// If we can't re-read, create a restore patch from original to original (identity)
					// This at least marks that we've seen this resource
					identityPatch, _ := jsonpatch.CreateMergePatch(original, original)
					if string(identityPatch) != "{}" {
						resourceWrapper.restorePatches[resource.GetName()] = string(identityPatch)
					}
				} else {
					// Re-read successful: create restore patch from current state to original
					currentState, err := json.Marshal(currentResource.Object)
					if err != nil {
						g.logger.Error(err, "failed to marshal current resource state",
							"resourceName", resource.GetName(),
							"resourceKind", resource.GetKind(),
						)
						continue
					}
					
					// Create restore patch from current state (might be modified) to original state
					restorePatchFromCurrent, err := jsonpatch.CreateMergePatch(currentState, original)
					if err != nil {
						g.logger.Error(err, "failed to create restore patch from current to original",
							"resourceName", resource.GetName(),
							"resourceKind", resource.GetKind(),
						)
						continue
					}
					
					restorePatchString := string(restorePatchFromCurrent)
					// Only save if it's not empty (resource was actually modified)
					if restorePatchString != "{}" {
						resourceWrapper.restorePatches[resource.GetName()] = restorePatchString
						g.logger.Info("saved restore patch from current state to original despite patch failure",
							"resourceName", resource.GetName(),
							"resourceKind", resource.GetKind(),
						)
					}
				}
				continue
			}

			// Patch succeeded: create proper restore patch (from modified back to original)
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

			// Save the restore patch (from modified to original)
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

		patcherFn, err := patcher.New([]byte(resourceWrapper.patchData.Patch))
		if err != nil {
			return fmt.Errorf("%w: %s", ErrJSONPatch, err)
		}

		for _, resource := range resourceWrapper.data {
			// Skip resources managed by another controller
			// EXCEPCIÓN: No saltar CRDs (PgBouncer, PgCluster, HDFSCluster) aunque tengan ownerReferences,
			// ya que estos son recursos de nivel superior que debemos gestionar directamente.
			resourceKind := resource.GetKind()
			isCRD := resourceKind == "PgBouncer" || resourceKind == "PgCluster" || resourceKind == "HDFSCluster"

			if metav1.GetControllerOfNoCopy(&resource) != nil && !isCRD {
				g.logger.Info("resource is managed by another controller, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resourceKind,
					"patch", resourceWrapper.patchData.Patch,
				)
				continue
			}

			current, err := json.Marshal(resource.Object)
			if err != nil {
				return fmt.Errorf("%w: %s", ErrJSONPatch, err)
			}

			// EXTENSIÓN PRIORITARIA: Para CRDs con patches dinámicos (PgCluster, HDFSCluster),
			// aplicar el patch de WAKE directamente sin verificar el restore patch.
			// Estos patches están diseñados para ser aplicados siempre, independientemente del estado del restore patch.
			isCRDWithDynamicPatch := resourceKind == "PgCluster" || resourceKind == "HDFSCluster"

			if isCRDWithDynamicPatch && resourceWrapper.patchData.Patch != "" {
				// Para CRDs con patches dinámicos, aplicar el patch directamente sin verificar restore patch
				g.logger.Info("applying dynamic patch for CRD (ignoring restore patch verification)",
					"resourceName", resource.GetName(),
					"resourceKind", resourceKind,
					"patch", resourceWrapper.patchData.Patch,
				)

				modified, err := patcherFn.Exec(current)
				if err != nil {
					// EXTENSIÓN: Manejar casos donde el patch falla por operación incorrecta
					// Los patches de WAKE usan "replace" pero si falla, intentar con "add"
					patchStr := resourceWrapper.patchData.Patch
					if strings.Contains(patchStr, "annotations") {
						if strings.Contains(patchStr, "op: replace") {
							// Si replace falla (anotación no existe, aunque debería), intentar con add
							patchStrAdd := strings.Replace(patchStr, "op: replace", "op: add", 1)
							fallbackPatcher, fallbackErr := patcher.New([]byte(patchStrAdd))
							if fallbackErr == nil {
								modified, err = fallbackPatcher.Exec(current)
								if err == nil {
									g.logger.V(8).Info("patch replace failed, successfully used add instead",
										"resourceName", resource.GetName(),
										"resourceKind", resourceKind,
									)
								}
							}
						}
					}

					if err != nil {
						g.logger.Error(err, "fails to apply dynamic patch",
							"resourceName", resource.GetName(),
							"resourceKind", resourceKind,
							"patch", resourceWrapper.patchData.Patch,
						)
						continue
					}
				}

				res := &unstructured.Unstructured{}
				if err := json.Unmarshal(modified, &res.Object); err != nil {
					return fmt.Errorf("%w: %s", ErrJSONPatch, err)
				}

				if err := resourceWrapper.SSAPatch(ctx, res); err != nil {
					return fmt.Errorf("%w: %s", ErrJSONPatch, err)
				}
				g.logger.Info("dynamic patch applied successfully for wake",
					"resourceName", resource.GetName(),
					"resourceKind", resourceKind,
				)
				resourceWrapper.isCacheInvalid = true
				continue
			}

			rawPatch, ok := resourceWrapper.restorePatches[resource.GetName()]
			if !ok {
				// EXTENSIÓN: Si no hay restore patch pero hay un patch definido, aplicar el patch directamente
				// Esto permite que SleepInfos de wake con patches funcionen sin necesidad de restore patches
				// (útil para recursos gestionados por operadores que se crean/eliminan dinámicamente)
				// IMPORTANTE: El operador restaurará las réplicas automáticamente basándose en el spec original del recurso
				if resourceWrapper.patchData.Patch != "" {
					g.logger.Info("no restore patch found, applying patch directly for wake",
						"resourceName", resource.GetName(),
						"resourceKind", resource.GetKind(),
						"patch", resourceWrapper.patchData.Patch,
					)

					modified, err := patcherFn.Exec(current)
					if err != nil {
						// EXTENSIÓN: Manejar casos donde el patch falla por operación incorrecta
						// - Si falla con "add" (anotación ya existe), intentar con "replace"
						// - Si falla con "replace" (anotación no existe), intentar con "add"
						patchStr := resourceWrapper.patchData.Patch
						if strings.Contains(patchStr, "annotations") {
							var fallbackPatcher *patcher.Patcher
							var fallbackErr error

							if strings.Contains(patchStr, "op: add") {
								// Intentar con replace si add falla
								patchStrReplace := strings.Replace(patchStr, "op: add", "op: replace", 1)
								fallbackPatcher, fallbackErr = patcher.New([]byte(patchStrReplace))
								if fallbackErr == nil {
									modified, err = fallbackPatcher.Exec(current)
									if err == nil {
										g.logger.V(8).Info("patch add failed, successfully used replace instead",
											"resourceName", resource.GetName(),
											"resourceKind", resource.GetKind(),
										)
									}
								}
							} else if strings.Contains(patchStr, "op: replace") {
								// Intentar con add si replace falla (anotación no existe)
								patchStrAdd := strings.Replace(patchStr, "op: replace", "op: add", 1)
								fallbackPatcher, fallbackErr = patcher.New([]byte(patchStrAdd))
								if fallbackErr == nil {
									modified, err = fallbackPatcher.Exec(current)
									if err == nil {
										g.logger.V(8).Info("patch replace failed, successfully used add instead",
											"resourceName", resource.GetName(),
											"resourceKind", resource.GetKind(),
										)
									}
								}
							}
						}

						if err != nil {
							g.logger.Error(err, "fails to apply patch (tried original and fallback)",
								"resourceName", resource.GetName(),
								"resourceKind", resource.GetKind(),
								"patch", resourceWrapper.patchData.Patch,
							)
							continue
						}
					}

					res := &unstructured.Unstructured{}
					if err := json.Unmarshal(modified, &res.Object); err != nil {
						return fmt.Errorf("%w: %s", ErrJSONPatch, err)
					}

					if err := resourceWrapper.SSAPatch(ctx, res); err != nil {
						return fmt.Errorf("%w: %s", ErrJSONPatch, err)
					}
					g.logger.Info("patch applied successfully for wake (operator will restore replicas from spec)",
						"resourceName", resource.GetName(),
						"resourceKind", resource.GetKind(),
					)
					resourceWrapper.isCacheInvalid = true
					continue
				}

				// Si no hay restore patch y no hay patch, omitir (comportamiento original para Deployments/StatefulSets)
				g.logger.Info("no restore patch found for resource, skipped",
					"resourceName", resource.GetName(),
					"resourceKind", resource.GetKind(),
				)
				continue
			}

			// Comportamiento original: usar restore patch si está disponible (solo para recursos nativos y PgBouncer)
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
