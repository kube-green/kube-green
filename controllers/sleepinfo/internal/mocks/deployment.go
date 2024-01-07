package mocks

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentOptions struct {
	Namespace       string
	Name            string
	Labels          map[string]string
	Replicas        *int32
	ResourceVersion string
	PodAnnotations  map[string]string
	MatchLabels     map[string]string
}

func Deployment(opts DeploymentOptions) ResourceMock[appsv1.Deployment] {
	matchLabels := opts.MatchLabels
	if matchLabels == nil {
		opts.MatchLabels = map[string]string{
			"app": opts.Name,
		}
	}
	return ResourceMock[appsv1.Deployment]{
		resource: appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            opts.Name,
				Namespace:       opts.Namespace,
				ResourceVersion: opts.ResourceVersion,
				Annotations:     opts.PodAnnotations,
				Labels:          opts.Labels,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: opts.MatchLabels,
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: opts.MatchLabels,
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "container",
								Image: "my-image",
							},
						},
					},
				},
				Replicas: opts.Replicas,
			},
		},
	}
}
