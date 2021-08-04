[![Go Report Card][go-report-svg]](go-report-card)
[![Test and build][test-and-build-svg]](test-and-build)
[![Coverage Status][coverage-badge]](coverage)

# Kube Green

How many development or demo namespaces remain active on weekends or beyond
business hours?  
This is an unnecessary waste of resources and money.

Kube Green was born with the aim of finding ways to avoid this kind of waste of
resources.

If you have some other ideas on how to improve our development on kubernetes,
please open an issue so we can integrate Kube Green features!

## Deploy

To install Kube Green in the cluster, clone the repository and run

```sh
make deploy
```

This will create a new namespace, kube-green, which contains the pod of the decorator.

## Usage

The use of this operator is very simple.

### SleepInfo

In the namespace where you want to enable Kube Green, apply the SleepInfo CRD.
An example of CRD is accessible [at this link](./testdata/working-hours.yml)

The SleepInfo spec contains:

* **weekdays**: day of the week. `*` is every day, `1` is monday, `1-5` is from monday to friday
* **sleepAt**: time in hours and minutes (HH:mm)when deployments replicas should be set to 0. Valid values are, for example, 19:00or `*:*` for every minute and every hour.
* **wakeUpAt**: time in hours and minutes (HH:mm)when deployments replicas should be set restored. Valid values are, for example, 19:00or `*:*` for every minute and every hour
* **timeZone**: time zone in IANA specification. For example for italian hour, set `Europe/Rome`
* **excluldeRef**: an array of object containing the resource to exclude from sleep. It contains:
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
  weekday: "1-5"
  sleepAt: "20:00"
  wakeUpAt: "08:00"
  timeZone: "Europe/Rome"
  excludeRef:
    - apiVersion: "apps/v1"
      kind:       Deployment
      name:       api-gateway
```

## Uninstall

To uninstall the operator from the cluster, run:

```sh
make undeploy
```

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
