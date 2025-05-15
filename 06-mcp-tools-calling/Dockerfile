FROM golang:1.24.2-alpine AS builder

WORKDIR /app
COPY main.go .
COPY go.mod .

RUN <<EOF
go mod tidy 
go build -o mcp-tools
EOF

#FROM scratch
FROM docker:cli
WORKDIR /app
COPY --from=builder /app/mcp-tools .

CMD ["./mcp-tools"]