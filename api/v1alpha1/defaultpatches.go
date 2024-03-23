package v1alpha1

var deploymentPatch = Patch{
	Target: PatchTarget{
		Group: "apps",
		Kind:  "Deployment",
	},
	Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
}

var cronjobPatch = Patch{
	Target: PatchTarget{
		Group: "batch",
		Kind:  "CronJob",
	},
	Patch: `
- op: add
  path: /spec/suspend
  value: true`,
}
