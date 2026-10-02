# Builds an Arrowhead core system binary.
# Build context: foundation/ directory

# Builder pinned by tag and digest; bumping it is a deliberate, tested change.
# Compiles on the build platform and cross-compiles for the target, so a
# multi-arch build needs no emulation for the Go build.
# Requires BuildKit (docker compose build, docker buildx build).
FROM --platform=$BUILDPLATFORM golang:1.25.14-alpine3.24@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder
ARG TARGETOS
ARG TARGETARCH
ENV GOTOOLCHAIN=local
ARG CMD
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /app ./cmd/${CMD}

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
