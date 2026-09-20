# Builds the pki-rest-authz service.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
COPY shared/authzforce/ /build/shared/authzforce/
COPY services/pki-rest-authz/ /build/services/pki-rest-authz/
WORKDIR /build/services/pki-rest-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9208 9209
ENTRYPOINT ["/app"]
