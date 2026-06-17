FROM docker.io/library/alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS builder

ARG TARGETPLATFORM

WORKDIR /workspace

COPY dist/${TARGETPLATFORM}/kube-green .
COPY LICENSE .

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/kube-green .
USER 65532:65532

ENTRYPOINT ["/kube-green"]
