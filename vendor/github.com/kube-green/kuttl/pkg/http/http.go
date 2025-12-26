package http

import (
	"bytes"
	"fmt"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kuttl/pkg/kubernetes"
)

// IsURL returns true if string is an URL
func IsURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// ToObjects takes a url, pulls the file and returns  []runtime.Object
// url must be a full path to a manifest file.  that file can have multiple runtime objects.
func ToObjects(urlPath string) ([]client.Object, error) {
	apply := []client.Object{}

	buf, err := Read(urlPath)
	if err != nil {
		return nil, err
	}

	objs, err := kubernetes.LoadYAML(urlPath, buf)
	if err != nil {
		return nil, fmt.Errorf("url %q load yaml error: %w", urlPath, err)
	}
	apply = append(apply, objs...)

	return apply, nil
}

// Read returns a buffer for the file at the url
func Read(urlPath string) (*bytes.Buffer, error) {
	c := NewClient()
	return c.GetByteBuffer(urlPath)
}
