# Builds the topic-auth-xacml service.
# Extended with connection-time cert-validity pre-gate (D2'): before consulting
# AuthzForce, handleUser and handleVhost query PIP directly. If certValid=false,
# the AMQP connection is rejected without calling AuthzForce at all.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
COPY shared/authzforce/ /build/shared/authzforce/
COPY services/topic-auth-xacml/ /build/services/topic-auth-xacml/
WORKDIR /build/services/topic-auth-xacml
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
