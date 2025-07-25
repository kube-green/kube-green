package mocks

import (
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type CronJobOptions struct {
	Namespace       string
	Name            string
	Labels          map[string]string
	ResourceVersion string
	Schedule        string
	Suspend         *bool
	Version         string
}

func CronJob(opts CronJobOptions) unstructured.Unstructured {
	if opts.Schedule == "" {
		opts.Schedule = "0 0 1-5 * *"
	}
	var cronJob interface{}
	switch opts.Version {
	case "v1beta1":
		cronJob = batchv1beta1.CronJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CronJob",
				APIVersion: "batch/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            opts.Name,
				Namespace:       opts.Namespace,
				ResourceVersion: opts.ResourceVersion,
				Labels:          opts.Labels,
			},
			Spec: batchv1beta1.CronJobSpec{
				Schedule: opts.Schedule,
				Suspend:  opts.Suspend,
				JobTemplate: batchv1beta1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:      opts.Name,
						Namespace: opts.Namespace,
					},
					Spec: batchv1.JobSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								RestartPolicy: v1.RestartPolicyNever,
								Containers: []v1.Container{
									{
										Name:  "c1",
										Image: "my-image",
									},
								},
							},
						},
					},
				},
			},
		}
	case "v1":
		fallthrough
	default:
		cronJob = batchv1.CronJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CronJob",
				APIVersion: "batch/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            opts.Name,
				Namespace:       opts.Namespace,
				ResourceVersion: opts.ResourceVersion,
				Labels:          opts.Labels,
			},
			Spec: batchv1.CronJobSpec{
				Schedule: opts.Schedule,
				Suspend:  opts.Suspend,
				JobTemplate: batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:      opts.Name,
						Namespace: opts.Namespace,
						Labels:    opts.Labels,
					},
					Spec: batchv1.JobSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								RestartPolicy: v1.RestartPolicyNever,
								Containers: []v1.Container{
									{
										Name:  "c1",
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
	unstructuredCronJob, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&cronJob)
	if err != nil {
		panic(err)
	}
	return unstructured.Unstructured{
		Object: unstructuredCronJob,
	}
}
