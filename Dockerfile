# Multi-stage build for kernl. Produces a small image that runs `kernl serve`
# (the API + embedded web UI) — an optional, simpler quickstart for users who
# just want the GUI without a local Go/Node toolchain.
#
# Note: the multi-agent orchestrator shells out to host tools (git, gh, the
# agent CLIs). Those are NOT in this image — it targets the graph/notes/serve
# experience. Full orchestration still runs on a host with the toolchain.

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
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git && adduser -D -u 10001 kernl
COPY --from=build /kernl /usr/local/bin/kernl
USER kernl
EXPOSE 8080
ENTRYPOINT ["kernl"]
CMD ["serve"]
