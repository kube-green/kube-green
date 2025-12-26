package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kuttl/pkg/kubernetes"
)

// from a list of paths, returns an array of runtime objects
func ToObjects(paths []string) ([]client.Object, error) {
	apply := []client.Object{}

	for _, path := range paths {
		objs, err := kubernetes.LoadYAMLFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("file %q load yaml error: %w", path, err)
		}
		apply = append(apply, objs...)
	}

	return apply, nil
}

// From a file or dir path returns an array of flat file paths
// pattern is a filepath.Match pattern to limit files to a pattern
func FromPath(path, pattern string) ([]string, error) {
	files := []string{}

	if pattern == "" {
		pattern = "*"
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file mode issue with %w", err)
	}
	if fi.IsDir() {
		fileInfos, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, fileInfo := range fileInfos {
			match, err := filepath.Match(pattern, fileInfo.Name())
			if err != nil {
				return nil, err
			}
			if !fileInfo.IsDir() && match {
				files = append(files, filepath.Join(path, fileInfo.Name()))
			}
		}
	} else {
		files = append(files, path)
	}

	return files, nil
}

// TrimExt removes the ext of a file path, foo.tar == foo
func TrimExt(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path))
}
