# AuthzForce server — reuses shared/authzforce-server.
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY shared/authzforce/ ./shared/authzforce/
COPY shared/authzforce-server/ ./shared/authzforce-server/
WORKDIR /src/shared/authzforce-server
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
