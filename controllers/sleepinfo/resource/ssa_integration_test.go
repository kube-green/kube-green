package resource

import (
	"context"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/davidebianchi/kube-green/controllers/internal/testutil"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Server side apply integration tests", func() {
	namespace := "test-namespace"
	var c ResourceClient

	It("create namespace", func() {
		Expect(testutil.CreateNamespace(context.Background(), k8sClient, namespace)).NotTo(HaveOccurred())
	})

	Describe("on namespace", func() {
		var resource unstructured.Unstructured

		BeforeEach(func() {
			resource = upsertResource(context.Background(), namespace)
			c = ResourceClient{
				SleepInfo:        &kubegreenv1alpha1.SleepInfo{},
				Log:              logr.DiscardLogger{},
				Client:           k8sClient,
				FieldManagerName: "kube-green-test",
			}
		})

		AfterEach(func() {
			cleanupNamespaceDeployments(k8sClient, namespace)
		})

		It("correctly patch resource", func() {
			newResource := resource.DeepCopy()
			newResource.SetLabels(map[string]string{
				"new-label": "new-value",
			})

			err := c.SSAPatch(context.Background(), newResource)
			Expect(err).NotTo(HaveOccurred())

			unstructuredRes := &unstructured.Unstructured{
				Object: resource.NewEmptyInstance().UnstructuredContent(),
			}
			err = testutil.GetResource(context.Background(), k8sClient, resource.GetName(), resource.GetNamespace(), unstructuredRes)
			Expect(err).NotTo(HaveOccurred())
			Expect(unstructuredRes).To(Equal(newResource))
		})

		It("correctly patch the same resource", func() {
			err := c.SSAPatch(context.Background(), &resource)
			Expect(err).NotTo(HaveOccurred())

			unstructuredRes := unstructured.Unstructured{
				Object: resource.NewEmptyInstance().UnstructuredContent(),
			}
			err = testutil.GetResource(context.Background(), k8sClient, resource.GetName(), resource.GetNamespace(), &unstructuredRes)
			Expect(err).NotTo(HaveOccurred())
			Expect(unstructuredRes).To(Equal(resource))
		})

		It("does not throw if resource not found", func() {
			resNotFound := unstructured.Unstructured{}
			resNotFound.SetName("not-exists")
			resNotFound.SetNamespace(namespace)
			resNotFound.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "apps",
				Kind:    "Deployment",
				Version: "v1",
			})
			err := c.SSAPatch(context.Background(), &resource)
			Expect(err).NotTo(HaveOccurred())

			res := unstructured.Unstructured{
				Object: resNotFound.NewEmptyInstance().UnstructuredContent(),
			}
			err = testutil.GetResource(context.Background(), k8sClient, resNotFound.GetName(), resNotFound.GetNamespace(), &res)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

})

func cleanupNamespaceDeployments(k8sClient client.Client, namespace string) {
	var (
		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)

	res := unstructured.Unstructured{}
	res.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})

	err := k8sClient.DeleteAllOf(context.Background(), &res, client.InNamespace(namespace))
	Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())

	newRes := unstructured.UnstructuredList{}
	newRes.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})

	Eventually(func() bool {
		err := k8sClient.List(context.Background(), &newRes)
		return len(newRes.Items) == 0 && err == nil
	}, timeout, interval).Should(BeTrue())
}

func upsertResource(ctx context.Context, namespace string) unstructured.Unstructured {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-name",
			Namespace: "test-namespace",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-name",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-name",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "my-container",
							Image: "my-image",
						},
					},
				},
			},
		},
	}

	resource, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
	Expect(err).NotTo(HaveOccurred())

	unstructuredResource := unstructured.Unstructured{
		Object: resource,
	}
	unstructuredResource.SetGroupVersionKind(schema.FromAPIVersionAndKind("apps/v1", "Deployment"))

	if err := k8sClient.Create(ctx, &deployment); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return unstructuredResource
}
