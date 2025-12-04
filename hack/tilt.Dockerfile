# Dockerfile (dev)
FROM ghcr.io/silogen/kaiwo-dev-base:0.0.1

WORKDIR /workspace

# Copy source code – this is the "thin" part rebuilt by Tilt

# Dev entrypoint: build + run the manager inside the container
COPY hack/dev-entrypoint.sh /usr/local/bin/dev-entrypoint.sh
RUN chmod +x /usr/local/bin/dev-entrypoint.sh

COPY cmd ./cmd
COPY apis ./apis
COPY pkg ./pkg
COPY internal ./internal

ENTRYPOINT ["/usr/local/bin/dev-entrypoint.sh"]
