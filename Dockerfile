# syntax=docker.io/docker/dockerfile@sha256:42399d4635eddd7a9b8a24be879d2f9a930d0ed040a61324cfdf59ef1357b3b2

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1@sha256:4918412049183afe42f1ecaf8f5c2a88917c2eab153ce5ecf4bf2d55c1507b74 AS golang

FROM golang AS build
WORKDIR /app
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
COPY go.??? .
RUN \
  --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
    set -ux \
 && GOOS=$TARGETOS GOARCH=$TARGETARCH go mod download
COPY . .
RUN \
  --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
    set -ux \
 && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags osusergo -o fmtd -ldflags '-s -w -extldflags "-static"' ./cmd

FROM scratch
COPY --from=build /app/fmtd /
