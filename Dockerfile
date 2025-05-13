# syntax=docker/dockerfile:1

FROM golang:1.24 AS builder
ARG REVISION
ENV CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build -trimpath -ldflags="-w -s -X main.revision=$REVISION" -o smtp2graph .

FROM registry.access.redhat.com/ubi9/ubi-micro
ARG REVISION
WORKDIR /app
COPY --from=builder /app/smtp2graph /app/smtp2graph
COPY --from=builder /app/README.md /app/README.md
COPY --from=builder /app/LICENSE /app/LICENSE

EXPOSE 1025
ENTRYPOINT ["/app/smtp2graph"]
