package utils

import (
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"runtime/pprof"
	"testing"
	"time"
)

// RunTests runs a Go test method without requiring the Go compiler.
// This does not currently support test caching.
// If testToRun is set to a non-empty string, it is passed as a `-run` argument to the go test harness.
// If paralellism is set, it limits the number of concurrently running tests.
func RunTests(testName string, testToRun string, parallelism int, testFunc func(*testing.T)) {
	flag.Parse()
	testing.Init()

	// Set the verbose test flag to true since we are not using the regular go test CLI.
	if err := flag.Set("test.v", "true"); err != nil {
		panic(err)
	}

	// Set the -run flag on the Go test harness.
	// See the go test documentation: https://golang.org/pkg/cmd/go/internal/test/
	if testToRun != "" {
		if err := flag.Set("test.run", fmt.Sprintf("//%s", testToRun)); err != nil {
			panic(err)
		}
	}

	parallelismStr := "8"
	if parallelism != 0 {
		parallelismStr = fmt.Sprintf("%d", parallelism)
	}
	if err := flag.Set("test.parallel", parallelismStr); err != nil {
		panic(err)
	}

	os.Exit(testing.MainStart(&testDeps{}, []testing.InternalTest{
		{
			Name: testName,
			F:    testFunc,
		},
	}, nil, nil, nil).Run())
}

// testDeps implements the testDeps interface for MainStart.
type testDeps struct{}

var matchPat string
var matchRe *regexp.Regexp

func (testDeps) MatchString(pat, str string) (result bool, err error) {
	if matchRe == nil || matchPat != pat {
		matchPat = pat
		matchRe, err = regexp.Compile(matchPat)

		if err != nil {
			return
		}
	}

	return matchRe.MatchString(str), nil
}

func (testDeps) SetPanicOnExit0(bool) {}

func (testDeps) StartCPUProfile(w io.Writer) error {
	return pprof.StartCPUProfile(w)
}

func (testDeps) StopCPUProfile() {
	pprof.StopCPUProfile()
}

func (testDeps) WriteProfileTo(name string, w io.Writer, debug int) error {
	return pprof.Lookup(name).WriteTo(w, debug)
}

func (testDeps) ImportPath() string {
	return ""
}

func (testDeps) StartTestLog(_ io.Writer) {}

func (testDeps) StopTestLog() error {
	return nil
}

func (testDeps) CoordinateFuzzing(time.Duration, int64, time.Duration, int64, int, []corpusEntry, []reflect.Type, string, string) error {
	return nil
}

func (testDeps) RunFuzzWorker(func(corpusEntry) error) error {
	return nil
}

func (testDeps) ReadCorpus(string, []reflect.Type) ([]corpusEntry, error) {
	return nil, nil
}

func (testDeps) CheckCorpus([]interface{}, []reflect.Type) error {
	return nil
}

func (testDeps) ResetCoverage() {

}

func (testDeps) SnapshotCoverage() {

}

func (testDeps) InitRuntimeCoverage() (mode string, tearDown func(string, string) (string, error), snapcov func() float64) {
	return
}

// corpusEntry is from the public go testing which references an internal structure.
// corpusEntry is an alias to the same type as internal/fuzz.CorpusEntry.
// We use a type alias because we don't want to export this type, and we can't
// import internal/fuzz from testing.
type corpusEntry = struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []interface{}
	Generation int
	IsSeed     bool
}
