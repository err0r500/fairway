FROM golang:1.24-bookworm AS builder

# Install FDB client headers and library (glibc-based, multi-arch)
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "arm64" ]; then ARCH=aarch64; else ARCH=amd64; fi && \
    wget https://github.com/apple/foundationdb/releases/download/7.4.5/foundationdb-clients_7.4.5-1_${ARCH}.deb && \
    dpkg -i foundationdb-clients_7.4.5-1_${ARCH}.deb && \
    rm foundationdb-clients_7.4.5-1_${ARCH}.deb

WORKDIR /build

# Copy entire repo (build context is repo root)
COPY . .

# Navigate to benchmark and build with CGO enabled (required for FDB bindings)
WORKDIR /build/examples/todo-bench
ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-I/usr/local/include"
ENV CGO_LDFLAGS="-L/usr/local/lib"
ENV GOWORK=off
RUN go mod download && \
    go build -o /benchmark .

FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates wget && \
    rm -rf /var/lib/apt/lists/*

# Install FDB 7.4.5 client library (multi-arch)
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "arm64" ]; then ARCH=aarch64; else ARCH=amd64; fi && \
    wget https://github.com/apple/foundationdb/releases/download/7.4.5/foundationdb-clients_7.4.5-1_${ARCH}.deb && \
    dpkg -i foundationdb-clients_7.4.5-1_${ARCH}.deb && \
    rm foundationdb-clients_7.4.5-1_${ARCH}.deb

COPY --from=builder /benchmark /benchmark

EXPOSE 8080

ENTRYPOINT ["/benchmark"]
