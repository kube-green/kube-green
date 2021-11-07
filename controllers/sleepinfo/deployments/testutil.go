package deployments

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockSpec struct {
	Namespace       string
	Name            string
	Replicas        *int32
	ResourceVersion string
}

func GetMock(opts MockSpec) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.Name,
			Namespace:       opts.Namespace,
			ResourceVersion: opts.ResourceVersion,
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "my-image",
						},
					},
				},
			},
			Replicas: opts.Replicas,
		},
	}
}
