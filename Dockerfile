FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (includes migrations/)
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# -----------------------
# Runtime image
# -----------------------
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary
COPY --from=builder /app/bin/api ./api

# Copy migrations into runtime image
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./api"]
