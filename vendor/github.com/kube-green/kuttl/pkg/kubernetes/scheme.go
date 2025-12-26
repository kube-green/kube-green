package kubernetes

import (
	"fmt"
	"os"
	"sync"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/kube-green/kuttl/pkg/apis"
)

// ensure that we only add to the scheme once.
var schemeLock sync.Once

// TODO (kensipe): need to consider options around AlwaysAdmin https://github.com/kudobuilder/kudo/pull/1420/files#r391449597

// Scheme returns an initialized Kubernetes Scheme.
func Scheme() *runtime.Scheme {
	schemeLock.Do(func() {
		if err := apis.AddToScheme(scheme.Scheme); err != nil {
			fmt.Printf("failed to add API resources to the scheme: %v", err)
			os.Exit(-1)
		}
		if err := v1.AddToScheme(scheme.Scheme); err != nil {
			fmt.Printf("failed to add V1 API extension resources to the scheme: %v", err)
			os.Exit(-1)
		}
		if err := v1beta1.AddToScheme(scheme.Scheme); err != nil {
			fmt.Printf("failed to add V1beta1 API extension resources to the scheme: %v", err)
			os.Exit(-1)
		}
	})

	return scheme.Scheme
}
