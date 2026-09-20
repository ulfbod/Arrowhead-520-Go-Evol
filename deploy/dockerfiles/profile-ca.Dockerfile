# Builds the profile-ca service (CA-as-PIP).
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
# core contains proto/certlifecycle generated Go code
COPY core/ /build/core/
COPY shared/authzforce/ /build/shared/authzforce/
COPY services/profile-ca/ /build/services/profile-ca/
WORKDIR /build/services/profile-ca
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8787 8788 8789
ENTRYPOINT ["/app"]
