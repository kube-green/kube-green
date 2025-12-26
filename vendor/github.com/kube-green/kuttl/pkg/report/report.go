package report

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// The structs below define the report output useful in either json or xml format.  The xml format and structs
// are junit xml compliant.  A number of resources were used but https://www.ibm.com/support/knowledgecenter/SSQ2R2_9.1.1/com.ibm.rsar.analysis.codereview.cobol.doc/topics/cac_useresults_junit.html
// was very useful.  As well as:  https://www.onlinetool.io/xmltogo/

// KUTTL is different than junit testing in that the test steps could be useful to have a report on.  There could be value in
// having a TestCaseStep struct providing step details.  Sticking with the JUnit standard for now.

// Type defines the report.type of report to create.
type Type string

const (
	// XML defines the xml Type.
	XML Type = "xml"
	// JSON defines the json Type.
	JSON Type = "json"
)

// Property are name/value pairs which can be provided in the report for things such as kuttl.version.
type Property struct {
	Name  string `xml:"name,attr" json:"name"`
	Value string `xml:"value,attr" json:"value"`
}

// Properties defines the collection of properties.
type Properties struct {
	Property []Property `xml:"property" json:"property,omitempty"`
}

// Failure defines a test failure.
type Failure struct {
	// Text provides detailed information regarding failure.  It supports multi-line output.
	Text string `xml:",chardata" json:"text,omitempty"`
	// Message provides the summary of the failure.
	Message string `xml:"message,attr" json:"message"`
	Type    string `xml:"type,attr" json:"type,omitempty"`
}

// Testcase is the finest grain level of reporting, it is the kuttl test (which contains steps).
type Testcase struct {
	// Classname is a junit thing, for kuttl it is the testsuite name.
	Classname string `xml:"classname,attr" json:"classname"`
	// Name is the name of the test (folder of test if not redefined by the TestStep).
	Name string `xml:"name,attr" json:"name"`
	// Timestamp is the time when this Testcase started.
	// This attribute is not in the mentioned XML schema (unlike Testsuite.Timestamp) but should be
	// gracefully ignored by readers who do not expect it.
	Timestamp time.Time `xml:"timestamp,attr" json:"timestamp"`
	// Time is the elapsed time of the test (and all of its steps).
	Time string `xml:"time,attr" json:"time"`
	// Assertions is the number of asserts and errors defined in the test.
	Assertions int `xml:"assertions,attr" json:"assertions,omitempty"`
	// Failure defines a failure in this Testcase.
	Failure *Failure `xml:"failure" json:"failure,omitempty"`

	// end is not reported.  It is used to calculate duration times for testcase and testsuite.
	end time.Time
}

// Testsuite is a collection of Testcase and is a summary of those details.
type Testsuite struct {
	// Tests is the number of Testcases in the collection.
	Tests int `xml:"tests,attr" json:"tests"`
	// Failures is the summary number of all failure in the collection testcases.
	Failures int `xml:"failures,attr" json:"failures"`
	// Timestamp is the time when this Testsuite started.
	Timestamp time.Time `xml:"timestamp,attr" json:"timestamp"`
	// Time is the duration of time for this Testsuite, this is tricky as tests run concurrently.
	// This is the elapse time between the start of the testsuite and the end of the latest testcase in the collection.
	Time string `xml:"time,attr" json:"time"`
	// Name is the kuttl test name.
	Name string `xml:"name,attr" json:"name"`
	// Properties which are specific to this suite.
	Properties *Properties `xml:"properties" json:"properties,omitempty"`
	// Testcases is a collection of test cases.
	Testcases []*Testcase `xml:"testcase" json:"testcase,omitempty"`
	// SubSuites is a collection of child test suites.
	SubSuites         []*Testsuite `xml:"testsuite" json:"testsuite,omitempty"`
	lock              sync.Mutex
	reportGranularity string
}

// Testsuites is a collection of Testsuite and defines the rollup summary of all stats.
type Testsuites struct {
	// XMLName is required to refine the name (or case of the name) in the root xml element.  Otherwise it adds no value and is ignored for json output.
	XMLName xml.Name `json:"-"`
	// Name is the name of the full set of tests which is possible to set in kuttl but is rarely used :)
	Name string `xml:"name,attr" json:"name"`
	// Tests is a summary value of the total number of tests for all testsuites.
	Tests int `xml:"tests,attr" json:"tests"`
	// Failures is a summary value of the total number of failures for all testsuites.
	Failures int `xml:"failures,attr" json:"failures"`
	// Time is the elapsed time of the entire suite of tests.
	Time string `xml:"time,attr" json:"time"`
	// Properties which are for the entire set of tests.
	Properties *Properties `xml:"properties" json:"properties,omitempty"`
	// Testsuite is a collection of test suites.
	Testsuite []*Testsuite `xml:"testsuite" json:"testsuite,omitempty"`
	// Failure defines a failure in this Testsuites. Not part of the Junit XML report standard, however it is needed to
	// communicate test infra failures, such as failed auth, or connection issues.
	Failure *Failure `xml:"failure" json:"failure,omitempty"`
	start   time.Time
	lock    sync.Mutex
}

// StepReporter is an interface for reporting status of a test step.
type StepReporter interface {
	Failure(message string, errors ...error)
	AddAssertions(i int)
}

// TestReporter is an interface for reporting status of a test.
// For each step, call Step and use the returned step reporter.
// Make sure to call Done when a test ends (preferably using defer).
type TestReporter interface {
	Step(stepName string) StepReporter
	Done()
}

// NewSuiteCollection returns the address of a newly created TestSuites
func NewSuiteCollection(name string) *Testsuites {
	start := time.Now()
	return &Testsuites{XMLName: xml.Name{Local: "testsuites"}, Name: name, start: start}
}

// NewSuite returns the address of a newly created TestSuite
func NewSuite(name string, reportGranularity string) *Testsuite {
	start := time.Now()
	return &Testsuite{Name: name, Timestamp: start, reportGranularity: reportGranularity}
}

// NewCase returns the address of a newly create Testcase
func NewCase(name string) *Testcase {
	start := time.Now()
	return &Testcase{Name: name, Timestamp: start}
}

// NewFailure returns the address of a newly created Failure
func NewFailure(msg string, errs []error) *Failure {
	f := &Failure{Message: msg}

	// the mental debate... when there are more than 1 errors, the most common case is
	// an assert of yaml that is incorrect.  the first error has the diff and the second has the specific
	// error that is interesting.  The diff can be so long... and the second error added to a concat string gets buried
	// in the noise.  Seems better to just see the reason and have the user look at test stdout for the larger context if desired.
	if len(errs) > 0 {
		f.Text = errs[len(errs)-1].Error()
	}
	return f
}

// AddTestcase adds a testcase to a suite, providing stats and calculations to both
func (ts *Testsuite) AddTestcase(testcase *Testcase) {
	// this is needed to calc elapse time of testsuite in a async work
	testcase.end = time.Now()
	elapsed := time.Since(testcase.Timestamp)
	ts.lock.Lock()
	defer ts.lock.Unlock()
	testcase.Time = fmt.Sprintf("%.3f", elapsed.Seconds())
	testcase.Classname = filepath.Base(ts.Name)

	ts.Testcases = append(ts.Testcases, testcase)
	sort.Slice(ts.Testcases, func(i, j int) bool {
		return ts.Testcases[i].Name < ts.Testcases[j].Name
	})
	ts.Tests++
	if testcase.Failure != nil {
		ts.Failures++
	}
}

// AddProperty adds a property to a testsuite
func (ts *Testsuite) AddProperty(property Property) {
	if ts.Properties == nil {
		ts.Properties = &Properties{Property: []Property{property}}
		return
	}
	if ts.Properties.Property == nil {
		ts.Properties.Property = []Property{property}
		return
	}
	ts.Properties.Property = append(ts.Properties.Property, property)
}

// NewSubSuite creates a new child suite and returns it.
func (ts *Testsuite) NewSubSuite(name string) *Testsuite {
	s := NewSuite(name, "")
	ts.lock.Lock()
	defer ts.lock.Unlock()
	ts.SubSuites = append(ts.SubSuites, s)
	// Ensure consistent ordering to make testing easier.
	sort.Slice(ts.SubSuites, func(i, j int) bool {
		return ts.SubSuites[i].Name < ts.SubSuites[j].Name
	})
	return s
}

// summarize sets counters and elapsed time attributes based on children.
// Returns the timestamp of the end of latest child.
func (ts *Testsuite) summarize() time.Time {
	end := ts.Timestamp
	for _, subSuite := range ts.SubSuites {
		subSuiteEnd := subSuite.summarize()
		if subSuiteEnd.After(end) {
			end = subSuiteEnd
		}
		ts.Tests += subSuite.Tests
		ts.Failures += subSuite.Failures
	}
	for _, testcase := range ts.Testcases {
		if testcase.end.After(end) {
			end = testcase.end
		}
	}
	elapsed := end.Sub(ts.Timestamp)
	ts.Time = fmt.Sprintf("%.3f", elapsed.Seconds())
	return end
}

func (ts *Testsuite) NewTestReporter(name string) TestReporter {
	switch ts.reportGranularity {
	case "test":
		tc := NewCase(name)
		return &testReporter{testCase: tc, suite: ts}
	default:
		subSuite := ts.NewSubSuite(name)
		return &testReporter{suite: subSuite}
	}
}

type stepReport struct {
	name       string
	failed     bool
	failureMsg string
	errors     []error
	assertions int
}

func (s *stepReport) Failure(message string, errors ...error) {
	s.failed = true
	s.failureMsg = message
	s.errors = append(s.errors, errors...)
}

func (s *stepReport) AddAssertions(i int) {
	s.assertions += i
}

func (s *stepReport) populate(testCase *Testcase) {
	if s.failed {
		testCase.Failure = NewFailure(s.failureMsg, s.errors)
	}
	testCase.Assertions += s.assertions
}

type testReporter struct {
	suite       *Testsuite
	stepReports []*stepReport
	testCase    *Testcase
}

func (r *testReporter) Step(stepName string) StepReporter {
	step := &stepReport{name: stepName}
	r.stepReports = append(r.stepReports, step)
	return step
}

func (r *testReporter) Done() {
	if r.testCase != nil {
		// Reporting with test granularity.
		for _, report := range r.stepReports {
			report.populate(r.testCase)
		}
		r.suite.AddTestcase(r.testCase)
		return
	}
	// Reporting with step granularity.
	for _, report := range r.stepReports {
		testCase := NewCase(report.name)
		report.populate(testCase)
		r.suite.AddTestcase(testCase)
	}
}

var _ TestReporter = (*testReporter)(nil)
var _ StepReporter = (*stepReport)(nil)

// AddTestSuite is a convenience method to add a testsuite to the collection in testsuites
func (ts *Testsuites) AddTestSuite(testsuite *Testsuite) {
	// testsuite is added prior to stat availability, stat management in the close of the testsuites
	ts.lock.Lock()
	defer ts.lock.Unlock()
	ts.Testsuite = append(ts.Testsuite, testsuite)
	// Ensure consistent ordering to make testing easier.
	sort.Slice(ts.Testsuite, func(i, j int) bool {
		return ts.Testsuite[i].Name < ts.Testsuite[j].Name
	})
}

// AddProperty adds a property to a testsuites
func (ts *Testsuites) AddProperty(property Property) {
	if ts.Properties == nil {
		ts.Properties = &Properties{Property: []Property{property}}
		return
	}
	if ts.Properties.Property == nil {
		ts.Properties.Property = []Property{property}
		return
	}
	ts.Properties.Property = append(ts.Properties.Property, property)
}

// Close closes the report and does all end stat calculations
func (ts *Testsuites) Close() {
	elapsed := time.Since(ts.start)
	ts.Time = fmt.Sprintf("%.3f", elapsed.Seconds())

	// async work makes this necessary (stats for each testsuite)
	for _, testsuite := range ts.Testsuite {
		testsuite.summarize()

		ts.Tests += testsuite.Tests
		ts.Failures += testsuite.Failures
	}
}

// latestEnd provides the time of the latest end out of the collection of testcases

// Report prints a report for TestSuites to the directory.  ftype == json | xml
func (ts *Testsuites) Report(dir, name string, ftype Type) error {
	ts.Close()

	err := ensureDir(dir)
	if err != nil {
		return err
	}

	// if a report is requested it is always created
	switch ftype {
	case XML:
		return writeXMLReport(dir, name, ts)
	case JSON:
		fallthrough
	default:
		return writeJSONReport(dir, name, ts)
	}
}

func ensureDir(dir string) error {
	if dir == "" {
		return nil
	}
	_, err := os.Stat(dir)
	// TODO log this, need to passing logger or have logger added to Testsuites
	// Create the folder to save the report if it doesn't exist
	if os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		//	no need for error check, it is always returned and handled by caller
	}
	return err
}

// SetFailure adds a failure to the TestSuites collection for startup failures in the test harness
func (ts *Testsuites) SetFailure(message string) {
	ts.Failure = &Failure{
		Message: message,
	}
}

func writeXMLReport(dir, name string, ts *Testsuites) error {
	file := filepath.Join(dir, fmt.Sprintf("%s.xml", name))
	xDoc, err := xml.MarshalIndent(ts, " ", "  ")
	if err != nil {
		return err
	}
	xmlStr := string(xDoc)
	//nolint:gosec
	return os.WriteFile(file, []byte(xmlStr), 0644)
}

func writeJSONReport(dir, name string, ts *Testsuites) error {
	file := filepath.Join(dir, fmt.Sprintf("%s.json", name))
	jDoc, err := json.MarshalIndent(ts, " ", "  ")
	if err != nil {
		return err
	}

	//nolint:gosec
	return os.WriteFile(file, jDoc, 0644)
}
