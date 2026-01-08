# -----------------------------
# Stage 1 — Build the Go binary
# -----------------------------
FROM golang:1.24 AS builder

# Build args for arch
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Working directory
WORKDIR /app

# Copy source code
COPY . .

# Build Go binary for the target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o worker ./cmd/wolf_worker

# -----------------------------
# Stage 2 — Minimal runtime image
# -----------------------------
FROM debian:bookworm-slim

WORKDIR /usr/local/bin

# Copy binary built for the architecture
COPY --from=builder /app/worker .

# Install dependencies
RUN apt-get update \
    && apt-get install -y clang \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY ./cmd/wolf_worker/stdc++.h /usr/include/c++/12/bits/stdc++.h

RUN clang++ -O1 -x c++-header /usr/include/c++/12/bits/stdc++.h -o /usr/include/c++/12/bits/stdc++.h.pch

# Run binary
ENTRYPOINT ["/usr/local/bin/worker"]
