package files

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	testutils "github.com/kube-green/kuttl/pkg/test/utils"
)

// testStepRegex contains one capturing group to determine the index of a step file.
var testStepRegex = regexp.MustCompile(`^(\d+)-(?:[^\.]+)(?:\.yaml)?$`)

// CollectTestStepFiles collects a map of test steps and their associated files
// from a directory.
func CollectTestStepFiles(dir string, logger testutils.Logger) (map[int64][]string, error) {
	testStepFiles := map[int64][]string{}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		index, err := getIndexFromFile(file.Name())
		if err != nil {
			return nil, err
		}
		if index < 0 {
			logger.Log("Ignoring", file.Name(), "as it does not match file name regexp:", testStepRegex.String())
			continue
		}

		if testStepFiles[index] == nil {
			testStepFiles[index] = []string{}
		}

		testStepPath := filepath.Join(dir, file.Name())

		if file.IsDir() {
			testStepDir, err := os.ReadDir(testStepPath)
			if err != nil {
				return nil, err
			}

			for _, testStepFile := range testStepDir {
				testStepFiles[index] = append(testStepFiles[index], filepath.Join(
					testStepPath, testStepFile.Name(),
				))
			}
		} else {
			testStepFiles[index] = append(testStepFiles[index], testStepPath)
		}
	}

	return testStepFiles, nil
}

// getIndexFromFile returns the index derived from fileName's prefix, ex. "01-foo.yaml" has index 1.
// If an index isn't found, -1 is returned.
func getIndexFromFile(fileName string) (int64, error) {
	matches := testStepRegex.FindStringSubmatch(fileName)
	if len(matches) != 2 {
		return -1, nil
	}

	i, err := strconv.ParseInt(matches[1], 10, 32)
	return i, err
}
