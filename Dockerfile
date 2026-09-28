# Build stage
FROM golang:1.27.1-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o tlc cmd/tlc/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates sqlite-libs

# Create non-root user
RUN addgroup -g 1000 tlc && \
    adduser -D -u 1000 -G tlc tlc

WORKDIR /home/tlc

# Copy binary from builder
COPY --from=builder /build/tlc /usr/local/bin/tlc

# Create necessary directories with proper permissions
RUN mkdir -p /home/tlc/.local/share/tlc \
             /home/tlc/.config/tlc \
             /home/tlc/.cache/tlc && \
    chown -R tlc:tlc /home/tlc

# Switch to non-root user
USER tlc

# Set XDG environment variables
ENV XDG_DATA_HOME=/home/tlc/.local/share
ENV XDG_CONFIG_HOME=/home/tlc/.config
ENV XDG_CACHE_HOME=/home/tlc/.cache

# Create volumes for persistent data
VOLUME ["/home/tlc/.local/share/tlc", "/home/tlc/.config/tlc"]

# Default command
ENTRYPOINT ["tlc"]
CMD ["--help"]
