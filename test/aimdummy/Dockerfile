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

LABEL com.amd.aim.hfToken.required=False \
      com.amd.aim.model.canonicalName="HuggingFaceTB/SmolLM2-135M" \
      com.amd.aim.model.recommendedDeployments="{'gpuModel': 'MI300X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'latency', 'description': 'Optimized for latency on MI300X using fp8 precision'}, {'gpuModel': 'MI300X', 'gpuCount': 1, 'precision': 'fp16', 'metric': 'latency', 'description': 'Unoptimized for latency on MI300X using fp16 precision', 'profileId': 'mi300x-fp16-latency-unopt'}, {'gpuModel': 'MI300X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'throughput', 'description': 'Optimized for throughput on MI300X using fp8 precision'}, {'gpuModel': 'MI325X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'latency', 'description': 'Optimized for latency on MI325X using fp8 precision', 'profileId': 'mi325x-fp8-latency'}, {'gpuModel': 'MI325X', 'gpuCount': 1, 'precision': 'fp8', 'metric': 'throughput', 'description': 'Optimized for throughput on MI325X using fp8 precision', 'profileId': 'mi325x-fp8-throughput'}" \
      com.amd.aim.model.source="hf://HuggingFaceTB/SmolLM2-135M" \
      com.amd.aim.model.tags="text-generation, chat, instruction" \
      com.amd.aim.model.variants="HuggingFaceTB/SmolLM2-135M" \
      com.amd.aim.release.notes="" \
      com.amd.aim.title="aimdummy-smollm2-135"




ENV PATH="/"
COPY --from=profile-builder /app/dry-run /
COPY --from=server-builder /app/server /

CMD ["server"]
