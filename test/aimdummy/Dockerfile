# Build profiler
FROM golang:1.25 AS profile-builder

WORKDIR /app

COPY profiler/ .

RUN go build -o dry-run

# Build server
FROM golang:1.25 AS server-builder

WORKDIR /app

COPY server/ .
ENV CGO_ENABLED=0
RUN go build -o server -ldflags="-extldflags=-static"

# Final stage
FROM scratch

ENV PATH="/"
COPY --from=profile-builder /app/dry-run /
COPY --from=server-builder /app/server /

CMD ["server"]
