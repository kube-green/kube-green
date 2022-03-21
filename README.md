[![Go Report Card][go-report-svg]][go-report-card]
[![Coverage][test-and-build-svg]][test-and-build]
[![Security][security-badge]][security-pipelines]
[![Test][test-badge]][test-pipelines]
[![Coverage Status][coverage-badge]][coverage]
[![Documentations][website-badge]][website]

<div align="center">
  <img src="https://github.com/kube-green/kube-green/raw/main/logo/logo.png" width="250" >
</div>

How many of your dev/preview pods stay on during weekends? Or at night? It's a waste of resources! And money! But fear not, *kube-green* is here to the rescue.

*kube-green* is a simple **k8s addon** that automatically **shuts down** (some of) your **resources** when you don't need them.

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See how to install the project on a live system in our [docs](https://kube-green.dev/docs/install/).

### Prerequisites

Make sure you have Go installed ([download](https://go.dev/dl/)). Version 1.17 or higher is required.

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

There are also some tests which run using [*kuttl*](https://kuttl.dev/). To install *kuttl* follow [this guide](https://kuttl.dev/docs/#install-kuttl-cli).

To run this tests, set up a Kubernetes cluster using [kind](https://kind.sigs.k8s.io/)

```sh
kind create cluster --name kube-green-testing
```

Build the docker image

```sh
make docker-test-build
```

and load the docker image to test

```sh
kind load docker-image kubegreen/kube-green:e2e-test --name kube-green-testing
```

After this, it's possible to start the tests (skipping the cluster deletion)

```sh
kubectl kuttl test --skip-cluster-delete
```

## Deployment

To deploy *kube-green* in live systems, follow the [docs](https://kube-green.dev/docs/install/).

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

1. `export TAG_VERSION=v{{NEW_VERSION_TO_TAG}}` where `{{NEW_VERSION_TO_TAG}}` should be replaced with the next version to upgrade. N.B.: version should include `v` as first char.
2. run `./hack/update-version.sh $TAG_VERSION`
3. run `make`
4. run `make bundle` to generate the bundle
5. add, commit and push
6. git tag $TAG_VERSION
7. `git push --tags origin $TAG_VERSION`

## API Reference documentation

API reference is automatically generated with [this tool](https://github.com/ahmetb/gen-crd-api-reference-docs). To generate it automatically, are added in api versioned folder a file `doc.go` with the content of file `groupversion_info.go` and a comment with `+genclient` in the `sleepinfo_types.go` file for the resource type.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details

## Acknowledgement

Special thanks to [JGiola](https://github.com/JGiola) for the tech review.

## Give a Star! ⭐

If you like or are using this project, please give it a star. Thanks!

[go-report-svg]: https://goreportcard.com/badge/github.com/kube-green/kube-green
[go-report-card]: https://goreportcard.com/report/github.com/kube-green/kube-green
[test-and-build-svg]: https://github.com/kube-green/kube-green/actions/workflows/test.yml/badge.svg
[test-and-build]: https://github.com/kube-green/kube-green/actions/workflows/test.yml
[coverage-badge]: https://coveralls.io/repos/github/kube-green/kube-green/badge.svg?branch=main
[coverage]: https://coveralls.io/github/kube-green/kube-green?branch=main
[website-badge]: https://img.shields.io/static/v1?label=kube-green&color=blue&message=docs&style=flat
[website]: https://kube-green.dev
[test-badge]: https://img.shields.io/github/workflow/status/kube-green/kube-green/Test%20and%20build?label=%F0%9F%A7%AA%20tests&style=flat
[test-pipelines]: https://github.com/kube-green/kube-green/actions/workflows/test.yml
[security-badge]: https://img.shields.io/github/workflow/status/kube-green/kube-green/Security?label=%F0%9F%94%91%20gosec&style=flat
[security-pipelines]: https://github.com/kube-green/kube-green/actions/workflows/security.yml
