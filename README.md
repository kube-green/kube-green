[![Go Report Card][go-report-svg]][go-report-card]
[![Test and build][test-and-build-svg]][test-and-build]
[![Coverage Status][coverage-badge]][coverage]

<div align="center">
  <img src="https://github.com/davidebianchi/kube-green/raw/main/logo/logo.png" width="250" >
</div>

How many of your dev/preview pods stay on during weekends? Or at night? It's a waste of resources! And money! But fear not, <br/> *kube-green* is here to the rescue.

*kube-green* is a simple k8s addon that automatically shuts down (some of) your resources when you don't need them.

Keep reading to find out how to use it, and if you have ideas on how to improve *kube-green*, open an issue, we'd love to hear them!

## Install

To install *kube-green* just clone this repository and run:

```sh
make deploy
```

This will create a new namespace, *kube-green*, which contains the pod of the operator.

For further information about the installation, [see here](docs/install.md)

## Usage

The use of this operator is very simple. Once installed on the cluster, configure the desired CRD to make it works.

### SleepInfo

In the namespace where you want to enable *kube-green*, apply the SleepInfo CRD.
An example of CRD is accessible [at this link](testdata/working-hours.yml)

The SleepInfo spec contains:

* **weekdays**: day of the week. `*` is every day, `1` is monday, `1-5` is from monday to friday
* **sleepAt**: time in hours and minutes (HH:mm)when deployments replicas should be set to 0. Valid values are, for example, 19:00or `*:*` for every minute and every hour.
* **wakeUpAt** (*optional*): time in hours and minutes (HH:mm) when deployments replicas should be restored. Valid values are, for example, 19:00or `*:*` for every minute and every hour. If wake up value is not set, pod in namespace will not be restored. So, you will need to deploy the initial namespace configuration to restore it.
* **timeZone** (*optional*): time zone in IANA specification. For example for italian hour, set `Europe/Rome`
* **excluldeRef** (*optional*): an array of object containing the resource to exclude from sleep. It contains:
  * **apiVersion**: version of the resource. Now it is supported *"apps/v1"*
  * **kind**: the kind of resource. Now it is supported *"Deployment"*
  * **name**: the name of the resource.

An example of a complete SleepInfo resource:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: working-hours
spec:
  weekdays: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "Europe/Rome"
  excludeRef:
    - apiVersion: "apps/v1"
      kind:       Deployment
      name:       api-gateway
```

#### Lifecycle

TODO

## Uninstall

To uninstall the operator from the cluster, run:

```sh
make undeploy
```

> :warning: If you run undeploy command, the namespace of kube-green (by default `kube-green`), will be deleted.

## Versioning

To upgrade the version:

1. `export VERSION=v{{NEW_VERSION_TO_TAG}}` where `{{NEW_VERSION_TO_TAG}}` should be replaced with the next version to upgrade. N.B.: version should include `v` as first char.
2. run `./scripts/update-version.sh $VERSION`
3. run `make`
4. add, commit and push
5. git tag $VERSION
6. `git push --tags origin $VERSION`

## Acknowledgement

Special thanks to [JGiola](https://github.com/JGiola) for the tech review.

[go-report-svg]: https://goreportcard.com/badge/github.com/davidebianchi/kube-green
[go-report-card]: https://goreportcard.com/report/github.com/davidebianchi/kube-green
[test-and-build-svg]: https://github.com/davidebianchi/kube-green/actions/workflows/test.yml/badge.svg
[test-and-build]: https://github.com/davidebianchi/kube-green/actions/workflows/test.yml
[coverage-badge]: https://coveralls.io/repos/github/davidebianchi/kube-green/badge.svg?branch=main
[coverage]: https://coveralls.io/github/davidebianchi/kube-green?branch=main
