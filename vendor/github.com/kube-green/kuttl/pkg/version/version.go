package version

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Info contains versioning information.
type Info struct {
	GitVersion string `json:"gitVersion"`
	GitCommit  string `json:"gitCommit"`
	BuildDate  string `json:"buildDate"`
	GoVersion  string `json:"goVersion"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

// String returns info as a human-friendly version string.
func (info Info) String() string {
	return info.GitVersion
}

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	// These variables typically come from -ldflags settings and in
	// their absence fallback to the settings in pkg/version/base.go
	// developer fallback for version

	// this only happens when running from a build.  Release runs ARE correct.
	if strings.Contains(gitVersion, "$Format") {
		// on dev box, lets use a env var for version
		gitVersion = os.Getenv("KUTTL_DEV_VERSION")
		if gitVersion == "" {
			gitVersion = "dev"
		}
		gitCommit = "dev"
		// TODO (kensipe): add debug message!
	}

	return Info{
		GitVersion: gitVersion,
		GitCommit:  gitCommit,
		BuildDate:  buildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// Version is an extension of semver.Version
type Version struct {
	*semver.Version
}

// CompareMajorMinor provides Compare results -1, 0, 1 for only the major and minor element
// of the semver, ignoring the patch or prerelease elements.   This is useful if you are looking
// for minVersion for example 1.15.6 is version 1.15 or higher.
func (v *Version) CompareMajorMinor(o *Version) int {
	if d := compareSegment(v.Major(), o.Major()); d != 0 {
		return d
	}
	if d := compareSegment(v.Minor(), o.Minor()); d != 0 {
		return d
	}
	return 0
}

// compares v1 against v2 resulting in -1, 0, 1 for less than, equal, greater than
func compareSegment(v1, v2 uint64) int {
	if v1 < v2 {
		return -1
	}
	if v1 > v2 {
		return 1
	}

	return 0
}

// New provides an instance of Version from a semver string
func New(v string) (*Version, error) {
	ver, err := semver.NewVersion(v)
	if err != nil {
		return nil, err
	}
	return FromSemVer(ver), nil
}

// FromGithubVersion provides a version parsed from github semver which starts with "v".
// v1.5.2 provides a sem version of 1.5.2
func FromGithubVersion(v string) (*Version, error) {
	return New(Clean(v))
}

// FromSemVer converts a semver.Version to our Version
func FromSemVer(v *semver.Version) *Version {
	return &Version{v}
}

// MustParse parses a given version and panics on error.
func MustParse(v string) *Version {
	return FromSemVer(semver.MustParse(v))
}

// Clean returns version without a prefixed v if it exists
func Clean(ver string) string {
	if strings.HasPrefix(ver, "v") {
		return ver[1:]
	}
	return ver
}
