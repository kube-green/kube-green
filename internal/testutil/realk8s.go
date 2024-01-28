package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func GetResource(ctx context.Context, k8sClient client.Client, name, namespace string, resource *unstructured.Unstructured) error {
	return k8sClient.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}, resource)
}

type nsKey struct{}

// CreateNamespace creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func CreateNamespace(ctx context.Context, cfg *envconf.Config, t *testing.T, runID string) (context.Context, error) {
	t.Helper()

	ns := envconf.RandomName(runID, 32)
	ctx = context.WithValue(ctx, nsKey{}, ns)

	cfg.WithNamespace(ns)

	t.Logf("Creating NS %v for test %v", ns, t.Name())
	nsObj := v1.Namespace{}
	nsObj.Name = ns
	nsObj.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "kube-green-test",
	})
	return ctx, cfg.Client().Resources().Create(ctx, &nsObj)
}

// DeleteNamespace looks up the namespace corresponding to the given test and deletes it.
func DeleteNamespace(ctx context.Context, cfg *envconf.Config, t *testing.T, _ string) (context.Context, error) {
	t.Helper()

	ns, ok := ctx.Value(nsKey{}).(string)
	if !ok {
		return ctx, fmt.Errorf("namespace not in context")
	}

	t.Logf("Deleting NS %v for test %v", ns, t.Name())
	nsObj := v1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Delete(ctx, &nsObj)
}

func GetClusterVersion() env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(c.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}

		info, err := discoveryClient.ServerVersion()
		if err != nil {
			return ctx, err
		}

		fmt.Printf("cluster version: %s\n", info.String())

		return ctx, nil
	}
}

func SetupCRDs(crdPath, pattern string) env.Func {
	return func(ctx context.Context, c *envconf.Config) (context.Context, error) {
		r, err := resources.New(c.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}
		h := decoder.CreateIgnoreAlreadyExists(r)
		return ctx, decoder.DecodeEachFile(ctx, os.DirFS(crdPath), pattern, h)
	}
}
