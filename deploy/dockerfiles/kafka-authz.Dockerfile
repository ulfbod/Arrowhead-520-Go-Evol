# Builds the kafka-authz service.
# Kafka-level connection enforcement is handled by ArrowheadPrincipalBuilder;
# kafka-authz continues to enforce message-level authorization.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
COPY shared/authzforce/ /build/shared/authzforce/
COPY services/kafka-authz/ /build/services/kafka-authz/
WORKDIR /build/services/kafka-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9101
ENTRYPOINT ["/app"]
