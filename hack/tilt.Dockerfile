# Dockerfile (dev)
FROM ghcr.io/silogen/kaiwo-dev-base:0.0.3

# Switch to root to install entrypoint (base image ends as USER 65532)
USER root

# Dev entrypoint: build + run the manager inside the container (cached layer)
COPY hack/dev-entrypoint.sh /usr/local/bin/dev-entrypoint.sh
RUN chmod +x /usr/local/bin/dev-entrypoint.sh

WORKDIR /workspace

# Copy source code – this is the "thin" part rebuilt by Tilt
COPY --chown=65532:65532 cmd ./cmd
COPY --chown=65532:65532 apis ./apis
COPY --chown=65532:65532 pkg ./pkg
COPY --chown=65532:65532 internal ./internal

# Switch back to non-root user
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/dev-entrypoint.sh"]
