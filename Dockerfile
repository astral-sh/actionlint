FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine3.23@sha256:e57c41c1d5864341031181b0db34b9a537bb5773eb6428e4e5bdaea0f9135406 AS builder
WORKDIR /src
COPY go.* *.go ./
COPY cmd cmd/
ARG TARGETOS
ARG TARGETARCH
ARG ACTIONLINT_VER=
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -mod=readonly \
    -ldflags "-s -w -X github.com/astral-sh/actionlint.version=${ACTIONLINT_VER}" \
    -o /out/${TARGETARCH}/actionlint ./cmd/actionlint

# Release builds replace this named context with the verified release binaries.
FROM builder AS binaries

FROM koalaman/shellcheck-alpine:v0.11.0@sha256:9955be09ea7f0dbf7ae942ac1f2094355bb30d96fffba0ec09f5432207544002 AS shellcheck
FROM ghcr.io/astral-sh/ruff:0.16.2@sha256:dc94041bbe2b1a4d846b00444c0b8d7160d44b7ed72339267263e9535e678ffb AS ruff

FROM alpine:3.23.5@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
ARG TARGETARCH
COPY --from=binaries /out/${TARGETARCH}/actionlint /usr/local/bin/actionlint
COPY --from=shellcheck /bin/shellcheck /usr/local/bin/shellcheck
COPY --from=ruff /ruff /usr/local/bin/ruff
LABEL org.opencontainers.image.source="https://github.com/astral-sh/actionlint" \
      org.opencontainers.image.licenses="MIT"
# Docker actions need the default user to access GITHUB_WORKSPACE. CLI users can
# choose an unprivileged user with docker run --user.
ENTRYPOINT ["/usr/local/bin/actionlint"]
