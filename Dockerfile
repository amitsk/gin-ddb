# syntax=docker/dockerfile:1
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

# Alpine provides busybox wget for the /health healthcheck.
FROM alpine:3.21 AS runtime
RUN adduser -D -u 65532 appuser
WORKDIR /app
COPY --from=build /out/api /app/api
USER appuser
EXPOSE 3000
ENV PORT=3000 HOST=0.0.0.0 DYNAMODB_TABLE_NAME=SkiLifts
ENTRYPOINT ["/app/api"]
