# Builds the cert-provisioner service.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY services/cert-provisioner/ ./services/cert-provisioner/
WORKDIR /src/services/cert-provisioner
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
