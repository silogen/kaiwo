# This code includes portions from vllm project, licensed under the Apache License 2.0.
# See https://github.com/vllm-project/vllm.git for details.

# Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

# http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
This example shows how to use Ray Data for data parallel batch inference.

Ray Data is a data processing framework that can handle large datasets
and integrates tightly with vLLM for data-parallel inference.

As of Ray 2.44, Ray Data has a native integration with
vLLM (under ray.data.llm).

Ray Data provides functionality for:
* Reading and writing to cloud storage (S3, GCS, etc.)
* Automatic sharding and load-balancing across a cluster
* Optimized configuration of vLLM using continuous batching
* Compatible with tensor/pipeline parallel inference as well.

Learn more about Ray Data's LLM integration:
https://docs.ray.io/en/latest/data/working-with-llms.html
"""
import os

import ray
from packaging.version import Version
from ray.data.llm import build_llm_processor, vLLMEngineProcessorConfig

assert Version(ray.__version__) >= Version("2.44.1"), "Ray version must be at least 2.44.1"

# Uncomment to reduce clutter in stdout
# ray.init(log_to_driver=False)
# ray.data.DataContext.get_current().enable_progress_bars = False

# Read one text file from S3. Ray Data supports reading multiple files
# from cloud storage (such as JSONL, Parquet, CSV, binary format).
ds = ray.data.read_text("s3://anonymous@air-example-data/prompts.txt")
print(ds.schema())

size = ds.count()
print(f"Size of dataset: {size} prompts")

# Configure vLLM engine.
config = vLLMEngineProcessorConfig(
    model_source=os.getenv("MODEL_ID", "meta-llama/Llama-3.1-8B-Instruct"),
    engine_kwargs={
        "enable_chunked_prefill": True,
        "max_num_batched_tokens": 4096,
        "max_model_len": 16384,
        "tensor_parallel_size": int(os.getenv("NUM_GPUS_PER_REPLICA", "8")),
        "distributed_executor_backend": "ray",
        "tokenizer_pool_size": 4,
        "tokenizer_pool_type": "ray",
        "trust_remote_code": True,
        "device": "cuda",
        "enforce_eager": False,
    },
    concurrency=int(os.getenv("NUM_REPLICAS", "1")),  # set the number of parallel vLLM replicas
    batch_size=64,
)

# Create a Processor object, which will be used to
# do batch inference on the dataset
vllm_processor = build_llm_processor(
    config,
    preprocess=lambda row: dict(
        messages=[
            {"role": "system", "content": "You are a bot that responds with haikus."},
            {"role": "user", "content": row["text"]},
        ],
        sampling_params=dict(
            temperature=0.3,
            max_tokens=250,
        ),
    ),
    postprocess=lambda row: dict(
        answer=row["generated_text"], **row  # This will return all the original columns in the dataset.
    ),
)

ds = vllm_processor(ds)

# Peek first 10 results.
# NOTE: This is for local testing and debugging. For production use case,
# one should write full result out as shown below.
outputs = ds.take(limit=10)

for output in outputs:
    prompt = output["prompt"]
    generated_text = output["generated_text"]
    print(f"Prompt: {prompt!r}")
    print(f"Generated text: {generated_text!r}")

# Write inference output data out as Parquet files to S3.
# Multiple files would be written to the output destination,
# and each task would write one or more files separately.
#
# ds.write_parquet("s3://<your-output-bucket>")
