FROM docker.io/library/alpine:3.23.2@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62 AS builder

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
