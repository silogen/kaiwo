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

LABEL org.amd.silogen.hfToken.required=False \
      org.amd.silogen.model.canonicalName="HuggingFaceTB/SmolLM2-135M" \
      org.amd.silogen.model.recommendedDeployments="{'gpuModel': 'MI300X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'latency', 'description': 'Optimized for latency on MI300X using fp8 precision'}, {'gpuModel': 'MI300X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'throughput', 'description': 'Optimized for throughput on MI300X using fp8 precision'}, {'gpuModel': 'MI325X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'latency', 'description': 'Optimized for latency on MI325X using fp8 precision'}, {'gpuModel': 'MI325X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'throughput', 'description': 'Optimized for throughput on MI325X using fp8 precision'}" \
      org.amd.silogen.model.source="hf://HuggingFaceTB/SmolLM2-135M" \
      org.amd.silogen.model.tags="text-generation, chat, instruction" \
      org.amd.silogen.model.variants="HuggingFaceTB/SmolLM2-135M" \
      org.amd.silogen.release.notes="" \
      org.amd.silogen.title="aimdummy-smollm2-135"




ENV PATH="/"
COPY --from=profile-builder /app/dry-run /
COPY --from=server-builder /app/server /

CMD ["server"]
