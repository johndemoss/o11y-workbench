FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /otel-workbench-web ./cmd/otel-workbench-web/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /otel-workbench-web /otel-workbench-web
EXPOSE 8080
ENTRYPOINT ["/otel-workbench-web"]
CMD ["--addr", ":8080"]
