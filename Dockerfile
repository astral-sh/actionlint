ARG GOLANG_VER=latest
ARG ALPINE_VER=latest

FROM golang:${GOLANG_VER} AS builder
WORKDIR /go/src/app
COPY go.* *.go ./
COPY cmd cmd/
ENV CGO_ENABLED=0
ARG ACTIONLINT_VER=
RUN go build -v -ldflags "-s -w -X github.com/astral-sh/actionlint.version=${ACTIONLINT_VER}" ./cmd/actionlint

FROM koalaman/shellcheck-alpine:stable AS shellcheck
FROM ghcr.io/astral-sh/ruff:0.16.2@sha256:dc94041bbe2b1a4d846b00444c0b8d7160d44b7ed72339267263e9535e678ffb AS ruff

FROM alpine:${ALPINE_VER}
COPY --from=builder /go/src/app/actionlint /usr/local/bin/
COPY --from=shellcheck /bin/shellcheck /usr/local/bin/shellcheck
COPY --from=ruff /ruff /usr/local/bin/ruff
USER 405:100
ENTRYPOINT ["/usr/local/bin/actionlint"]
