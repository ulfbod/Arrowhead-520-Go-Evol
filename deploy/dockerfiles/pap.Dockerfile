# Builds the PAP (Policy Administration Point).
# Build context: repo root (Arrowhead-520-evol/)

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build/services/pap
COPY shared/ /build/shared/
COPY services/pap/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9505
ENTRYPOINT ["/app"]
