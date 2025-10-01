# syntax=docker/dockerfile:1.6

FROM golang:1.24 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/ingest ./cmd/ingest && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/mcp-server ./cmd/mcp-server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/dbstatus ./cmd/dbstatus

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=builder /workspace/config-go.env /app/config-go.env
COPY --from=builder /workspace/dist/ingest /usr/local/bin/ingest
COPY --from=builder /workspace/dist/mcp-server /usr/local/bin/mcp-server
COPY --from=builder /workspace/dist/dbstatus /usr/local/bin/dbstatus

ENV CONFIG_PATH=/app/config-go.env

EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/mcp-server"]

