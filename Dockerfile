FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mocktracegen-web ./cmd/mocktracegen-web/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /mocktracegen-web /mocktracegen-web
EXPOSE 8080
ENTRYPOINT ["/mocktracegen-web"]
CMD ["--addr", ":8080"]
