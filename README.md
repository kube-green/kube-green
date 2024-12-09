[![Go Report Card][go-report-svg]][go-report-card]
[![Coverage][test-and-build-svg]][test-and-build]
[![Security][security-badge]][security-pipelines]
[![Coverage Status][coverage-badge]][coverage]
[![Documentations][website-badge]][website]
[![Adopters][adopters-badge]][adopters]
[![CNCF Landscape][cncf-badge]][cncf-landscape]

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/kube-green/kube-green/main/logo/logo-horizontal-dark.svg">
  <img alt="Dark kube-green logo" src="https://raw.githubusercontent.com/kube-green/kube-green/main/logo/logo-horizontal.svg">
</picture>

How many of your dev/preview pods stay on during weekends? Or at night? It's a waste of resources! And money! But fear not, *kube-green* is here to the rescue.

*kube-green* is a simple **k8s addon** that automatically **shuts down** (some of) your **resources** when you don't need them.

If you already use *kube-green*, add you as an [adopter][add-adopters]!

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See how to install the project on a live system in our [docs](https://kube-green.dev/docs/install/).

### Prerequisites

Make sure you have Go installed ([download](https://go.dev/dl/)). Version 1.19 or higher is required.

## Installation

To have *kube-green* running locally just clone this repository and install the dependencies running:

```golang
go get
```

## Running the tests

There are different types of tests in this repository.

It is possible to run all the unit tests with

```sh
make test
```

To run integration tests installing kube-green with kustomize, run:

```sh
make e2e-test-kustomize
```

otherwise, to run integration tests installing kube-green with helm, run:

```sh
make e2e-test
```

It is possible to run only a specific harness integration test, running e2e-test with the OPTION variable:

```sh
make e2e-test OPTION="-run=TestSleepInfoE2E/kuttl/run_e2e_tests/harness/{TEST_NAME}"
```

## Deployment

To deploy *kube-green* in live systems, follow the [docs](https://kube-green.dev/docs/install/).

To run kube-green for development purpose, you can use [ko](https://ko.build/) to deploy
in a KinD cluster.
It is possible to start a KinD cluster running `kind create cluster --name kube-green-development`.
To deploy kube-green using ko, run:

```sh
make local-run clusterName=kube-green-development
```

## Usage

The use of this operator is very simple. Once installed on the cluster, configure the desired CRD to make it works.

See [here](https://kube-green.dev/docs/configuration/) the documentation about the configuration of the CRD.

### CRD Examples

Pods running during working hours with Europe/Rome timezone, suspend CronJobs and exclude a deployment named `api-gateway`:

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
  suspendCronJobs: true
  excludeRef:
    - apiVersion: "apps/v1"
      kind:       Deployment
      name:       api-gateway
```

Pods sleep every night without restore:

```yaml
apiVersion: kube-green.com/v1alpha1
kind: SleepInfo
metadata:
  name: working-hours-no-wakeup
spec:
  sleepAt: "20:00"
  timeZone: Europe/Rome
  weekdays: "*"
```

To see other examples, go to [our docs](https://kube-green.dev/docs/configuration/#examples).

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/PurpleBooth/b24679402957c63ec426) for details on our code of conduct, and the process for submitting pull requests to us.

## Versioning

We use [SemVer](http://semver.org/) for versioning. For the versions available, see the [release on this repository](https://github.com/kube-green/kube-green/releases).

### How to upgrade the version

To upgrade the version:

1. `make release version=v{{NEW_VERSION_TO_TAG}}` where `{{NEW_VERSION_TO_TAG}}` should be replaced with the next version to upgrade. N.B.: version should include `v` as first char.
2. `git push --tags origin v{{NEW_VERSION_TO_TAG}}`

## API Reference documentation

API reference is automatically generated with [this tool](https://github.com/ahmetb/gen-crd-api-reference-docs). To generate it automatically, are added in api versioned folder a file `doc.go` with the content of file `groupversion_info.go` and a comment with `+genclient` in the `sleepinfo_types.go` file for the resource type.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details

## Acknowledgement

Special thanks to [JGiola](https://github.com/JGiola) for the tech review.

## Give a Star! ⭐

If you like or are using this project, please give it a star. Thanks!

## Adopters

[Here](https://kube-green.dev/docs/adopters/) the list of adopters of *kube-green*.

If you already use *kube-green*, add you as an [adopter][add-adopters]!

[go-report-svg]: https://goreportcard.com/badge/github.com/kube-green/kube-green
[go-report-card]: https://goreportcard.com/report/github.com/kube-green/kube-green
[test-and-build-svg]: https://github.com/kube-green/kube-green/actions/workflows/test.yml/badge.svg
[test-and-build]: https://github.com/kube-green/kube-green/actions/workflows/test.yml
[coverage-badge]: https://coveralls.io/repos/github/kube-green/kube-green/badge.svg?branch=main
[coverage]: https://coveralls.io/github/kube-green/kube-green?branch=main
[website-badge]: https://img.shields.io/static/v1?label=kube-green&color=blue&message=docs&style=flat
[website]: https://kube-green.dev
[security-badge]: https://github.com/kube-green/kube-green/actions/workflows/security.yml/badge.svg
[security-pipelines]: https://github.com/kube-green/kube-green/actions/workflows/security.yml
[adopters-badge]: https://img.shields.io/static/v1?label=ADOPTERS&color=blue&message=docs&style=flat
[adopters]: https://kube-green.dev/docs/adopters/
[add-adopters]: https://github.com/kube-green/kube-green.github.io/blob/main/CONTRIBUTING.md#add-your-organization-to-adopters
[cncf-badge]: https://img.shields.io/badge/CNCF%20Landscape-5699C6
[cncf-landscape]: https://landscape.cncf.io/?item=orchestration-management--scheduling-orchestration--kube-green
