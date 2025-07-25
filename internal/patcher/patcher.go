package patcher

import (
	jsonpatch "github.com/evanphx/json-patch/v5"
	"sigs.k8s.io/yaml"
)

type Patcher struct {
	patch jsonpatch.Patch
}

func (p Patcher) Exec(original []byte) ([]byte, error) {
	return p.patch.ApplyWithOptions(original, &jsonpatch.ApplyOptions{
		EnsurePathExistsOnAdd:    true,
		AllowMissingPathOnRemove: true,
	})
}

func (p Patcher) IsResourceChanged(original []byte) (bool, error) {
	modified, err := p.Exec(original)
	if err != nil {
		return false, err
	}

	return !jsonpatch.Equal(original, modified), nil
}

func New(patchToApply []byte) (*Patcher, error) {
	jsonPatchToApply, err := yaml.YAMLToJSON(patchToApply)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.DecodePatch(jsonPatchToApply)
	if err != nil {
		return nil, err
	}

	return &Patcher{
		patch: patch,
	}, nil
}
