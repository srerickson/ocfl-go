FROM golang:1.23 AS builder
WORKDIR /app
COPY . ./
RUN go mod download
RUN go build -o ./ocfl ./cmd/ocfl

FROM ubuntu:latest
COPY --from=builder /app/ocfl /usr/local/bin/ocfl