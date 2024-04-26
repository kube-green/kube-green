package v1alpha1

var DeploymentTarget = PatchTarget{
	Group: "apps",
	Kind:  "Deployment",
}

var CronJobTarget = PatchTarget{
	Group: "batch",
	Kind:  "CronJob",
}

var deploymentPatch = Patch{
	Target: DeploymentTarget,
	Patch: `
- op: add
  path: /spec/replicas
  value: 0`,
}

var cronjobPatch = Patch{
	Target: CronJobTarget,
	Patch: `
- op: add
  path: /spec/suspend
  value: true`,
}
