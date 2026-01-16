# Docker Guide for TLC

This guide covers running TLC in Docker containers for isolated, reproducible environments.

## Quick Start

### Using Docker

```bash
# Build the image
docker build -t tlc-cli .

# Run the TUI
docker run -it --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  -v tlc-config:/home/tlc/.config/tlc \
  tlc-cli tui
```

### Using Docker Compose

```bash
# Start the service
docker-compose up -d

# Access the TUI
docker-compose exec tlc tlc tui

# Run commands
docker-compose run --rm tlc task list
docker-compose run --rm tlc task create "New task" --tag feature

# Stop the service
docker-compose down
```

## Image Details

### Multi-stage Build
The Dockerfile uses a multi-stage build for optimal image size:
- **Builder stage**: Uses `golang:1.25.5-alpine` with build dependencies
- **Runtime stage**: Uses minimal `alpine:latest` with only runtime dependencies

### Security Features
- Runs as non-root user (`tlc:tlc` with UID/GID 1000)
- Minimal attack surface with Alpine Linux
- Static binary compilation with CGO for SQLite support

### Storage
The image creates volumes for persistent data:
- `/home/tlc/.local/share/tlc` - Task database and todo.txt
- `/home/tlc/.config/tlc` - Configuration files

## Common Use Cases

### Interactive TUI

```bash
docker run -it --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  -v tlc-config:/home/tlc/.config/tlc \
  tlc-cli tui
```

### One-off Commands

```bash
# Initialize a new project
docker run --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli init

# List tasks
docker run --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli task list

# Create a task
docker run --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli task create "Fix bug" --tag bug --priority high
```

### Custom Configuration

Mount a local configuration file:

```bash
docker run -it --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  -v ./tlc-config.yaml:/home/tlc/.config/tlc/config.yaml:ro \
  tlc-cli tui
```

Or use environment variables:

```bash
docker run -it --rm \
  -v tlc-data:/home/tlc/.local/share/tlc \
  -e TLC_LOG_LEVEL=debug \
  -e TLC_ARCHIVE_THRESHOLD=336h \
  tlc-cli tui
```

### Direct File Access

Mount the todo.txt file for direct editing:

```bash
docker run -it --rm \
  -v $(pwd)/todo.txt:/home/tlc/.local/share/tlc/todo.txt \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli task list
```

This allows you to edit `todo.txt` with your local editor while keeping the database in sync.

## Docker Compose Configuration

The included `docker-compose.yml` provides a ready-to-use configuration:

```yaml
version: '3.8'

services:
  tlc:
    build:
      context: .
      dockerfile: Dockerfile
    image: tlc-cli:latest
    container_name: tlc
    stdin_open: true
    tty: true
    volumes:
      - tlc-data:/home/tlc/.local/share/tlc
      - tlc-config:/home/tlc/.config/tlc
    environment:
      - TERM=xterm-256color
    command: ["tui"]

volumes:
  tlc-data:
  tlc-config:
```

### Customizing docker-compose.yml

Uncomment these sections in `docker-compose.yml` to enable:

```yaml
# Mount a local config file
- ./tlc-local.yaml:/home/tlc/.config/tlc/config.yaml:ro

# Mount a local todo.txt for direct editing
- ./todo.txt:/home/tlc/.local/share/tlc/todo.txt

# Set environment variables
TLC_LOG_LEVEL: "debug"
TLC_ARCHIVE_THRESHOLD: "168h"
```

## Volume Management

### List volumes
```bash
docker volume ls | grep tlc
```

### Inspect volume
```bash
docker volume inspect tlc-data
```

### Backup data
```bash
# Create a backup
docker run --rm \
  -v tlc-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/tlc-backup.tar.gz -C /data .

# Restore from backup
docker run --rm \
  -v tlc-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "cd /data && tar xzf /backup/tlc-backup.tar.gz"
```

### Remove volumes
```bash
# Stop containers first
docker-compose down

# Remove volumes
docker volume rm tlc-data tlc-config
```

## Building from Source

### Standard build
```bash
docker build -t tlc-cli:latest .
```

### Build with specific tag
```bash
docker build -t tlc-cli:v1.0.0 .
```

### Build with build args
```bash
docker build \
  --build-arg GO_VERSION=1.25.5 \
  -t tlc-cli:latest .
```

## Troubleshooting

### Container exits immediately
Ensure you're using `-it` flags for interactive mode:
```bash
docker run -it tlc-cli tui
```

### Permission issues
The container runs as UID/GID 1000. If you mount host directories, ensure they're writable:
```bash
chown -R 1000:1000 ./data
```

### Database locked errors
This can occur if multiple containers access the same volume simultaneously. Ensure only one container accesses the data at a time.

### TUI display issues
Set the TERM environment variable:
```bash
docker run -it --rm \
  -e TERM=xterm-256color \
  -v tlc-data:/home/tlc/.local/share/tlc \
  tlc-cli tui
```

## CI/CD Integration

### GitHub Actions example

```yaml
name: Docker Build

on:
  push:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build Docker image
        run: docker build -t tlc-cli:${{ github.sha }} .

      - name: Test the image
        run: |
          docker run --rm tlc-cli:${{ github.sha }} version
          docker run --rm tlc-cli:${{ github.sha }} --help
```

## Multi-architecture Builds

Build for multiple architectures using buildx:

```bash
# Create a builder
docker buildx create --name tlc-builder --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -t tlc-cli:latest \
  --push .
```

## Registry Publishing

### Docker Hub

```bash
# Tag the image
docker tag tlc-cli:latest yourusername/tlc-cli:latest
docker tag tlc-cli:latest yourusername/tlc-cli:v1.0.0

# Push to Docker Hub
docker push yourusername/tlc-cli:latest
docker push yourusername/tlc-cli:v1.0.0
```

### GitHub Container Registry

```bash
# Login
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Tag and push
docker tag tlc-cli:latest ghcr.io/yourusername/tlc-cli:latest
docker push ghcr.io/yourusername/tlc-cli:latest
```
