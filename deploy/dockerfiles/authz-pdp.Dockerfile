# Builds the authz-pdp gRPC server.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
COPY shared/authzforce/ /build/shared/authzforce/
COPY core/ /build/core/
WORKDIR /build/core
RUN CGO_ENABLED=0 go build -o /app ./cmd/authz-pdp

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9550
ENTRYPOINT ["/app"]
