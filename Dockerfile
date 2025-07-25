FROM docker.io/library/alpine:3.22.1@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1 AS builder

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
