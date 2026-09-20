# Builds an Arrowhead core system binary.
# Build context: foundation/ directory

FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto
ARG CMD
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/${CMD}

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
