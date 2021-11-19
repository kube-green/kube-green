package cronjobs

import (
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockSpec struct {
	Namespace       string
	Name            string
	ResourceVersion string
	Schedule        string
	Suspend         *bool
}

func GetMock(opts MockSpec) batchv1.CronJob {
	return batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CronJob",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.Name,
			Namespace:       opts.Namespace,
			ResourceVersion: opts.ResourceVersion,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: opts.Schedule,
			Suspend:  opts.Suspend,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opts.Name,
					Namespace: opts.Namespace,
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "my-image",
								},
							},
						},
					},
				},
			},
		},
	}
}
