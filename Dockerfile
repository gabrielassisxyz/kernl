# Multi-stage build for kernl. Produces an image that runs `kernl serve`
# (the API + embedded web UI) and includes the beads orchestrator (bd).
#
# Note: full multi-agent orchestration also requires agent CLIs (opencode, etc.)
# which are NOT in this image by default.

# 1. Build the web UI (embedded into the Go binary).
FROM node:24-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run generate

# 2. Compile the static Go binary (CGO off — modernc.org/sqlite is pure Go).
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/.output/public ./web/.output/public
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /kernl ./cmd/kernl

# 3. Minimal runtime.
FROM debian:bookworm-slim
ARG DOLT_VERSION=2.1.7

# Install system dependencies, beads (bd) and dolt
RUN apt-get update && apt-get install -y \
    ca-certificates \
    git \
    curl \
    bash \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash \
    && curl -fsSL https://github.com/dolthub/dolt/releases/download/v${DOLT_VERSION}/install.sh | bash \
    && useradd -m -u 10001 kernl \
    && mkdir -p /home/kernl/.kernl \
    && chown -R kernl:kernl /home/kernl

WORKDIR /home/kernl

COPY --from=build /kernl /usr/local/bin/kernl
USER kernl
EXPOSE 8080
ENTRYPOINT ["kernl"]
CMD ["serve"]
