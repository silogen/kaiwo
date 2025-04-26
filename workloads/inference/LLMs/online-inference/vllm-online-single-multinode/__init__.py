# This code includes portions from Ray project, licensed under the Apache License 2.0.
# See https://github.com/ray-project/ray.git for details.

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


import logging
import os
from typing import Dict, Optional

from fastapi import FastAPI
from ray import serve
from starlette.requests import Request
from starlette.responses import JSONResponse, StreamingResponse
from vllm.engine.arg_utils import AsyncEngineArgs
from vllm.engine.async_llm_engine import AsyncLLMEngine
from vllm.entrypoints.logger import RequestLogger
from vllm.entrypoints.openai.protocol import (
    ChatCompletionRequest,
    ChatCompletionResponse,
    ErrorResponse,
)
from vllm.entrypoints.openai.serving_chat import OpenAIServingChat

logger = logging.getLogger("ray.serve")

app = FastAPI()


@serve.deployment()
@serve.ingress(app)
class VLLMDeployment:
    def __init__(
        self,
        engine_args: AsyncEngineArgs,
        response_role: str,
        request_logger: Optional[RequestLogger] = None,
        chat_template: Optional[str] = None,
        chat_template_content_format: Optional[str] = "openai",
    ):
        logger.info(f"Starting with engine args: {engine_args}")
        self.openai_serving_chat = None
        self.engine_args = engine_args
        self.response_role = response_role
        self.request_logger = request_logger
        self.chat_template = chat_template
        self.chat_template_content_format = chat_template_content_format
        self.engine = AsyncLLMEngine.from_engine_args(engine_args)

    @app.post("/v1/chat/completions")
    async def create_chat_completion(self, request: ChatCompletionRequest, raw_request: Request):
        """OpenAI-compatible HTTP endpoint.

        API reference:
            - https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html
        """
        if not self.openai_serving_chat:
            model_config = await self.engine.get_model_config()
            # Determine the name of the served model for the OpenAI client.
            if self.engine_args.served_model_name is not None:
                served_model_names = self.engine_args.served_model_name
            else:
                served_model_names = [self.engine_args.model]
            self.openai_serving_chat = OpenAIServingChat(
                self.engine,
                model_config,
                served_model_names,
                self.response_role,
                lora_modules=self.lora_modules,
                prompt_adapters=self.prompt_adapters,
                request_logger=self.request_logger,
                chat_template=self.chat_template,
                chat_template_content_format=self.chat_template_content_format,
            )
        logger.info(f"Request: {request}")
        generator = await self.openai_serving_chat.create_chat_completion(request, raw_request)
        if isinstance(generator, ErrorResponse):
            return JSONResponse(content=generator.model_dump(), status_code=generator.code)
        if request.stream:
            return StreamingResponse(content=generator, media_type="text/event-stream")
        else:
            assert isinstance(generator, ChatCompletionResponse)
            return JSONResponse(content=generator.model_dump())


def build_app(cli_args: Dict[str, str]) -> serve.Application:
    """Builds the Serve app based on CLI arguments.

    See https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html#command-line-arguments-for-the-server
    for the complete set of arguments.

    Supported engine arguments: https://docs.vllm.ai/en/latest/models/engine_args.html.
    """  # noqa: E501
    engine_args = AsyncEngineArgs(
        model=os.getenv("MODEL_ID", "meta-llama/Meta-Llama-3-8B-Instruct"),
        disable_log_requests=True,
        dtype="auto",
        device="cuda",
        enable_chunked_prefill=False,
        enable_prefix_caching=True,
        gpu_memory_utilization=float(os.getenv("GPU_MEMORY_UTILIZATION", "0.8")),
        max_model_len=int(os.getenv("MAX_MODEL_LEN", "4096")),
        max_num_batched_tokens=int(os.getenv("MAX_NUM_BATCHED_TOKENS", "32768")),
        max_num_seqs=int(os.getenv("MAX_NUM_SEQ", "512")),
        pipeline_parallel_size=int(os.getenv("NUM_REPLICAS", "2")),
        tensor_parallel_size=int(os.getenv("NUM_GPUS_PER_REPLICA", "8")),
        tokenizer_pool_size=4,
        tokenizer_pool_type="ray",
        distributed_executor_backend="ray",
        trust_remote_code=True,
    )
    engine_args.worker_use_ray = True

    return VLLMDeployment.bind(
        engine_args=engine_args,
        response_role="assistant",
        lora_modules=None,
        prompt_adapters=None,
        request_logger=None,
        chat_template=None,
    )


deployment = build_app(
    {
        "model": os.environ["MODEL_ID"],
        "tensor-parallel-size": os.environ["TENSOR_PARALLELISM"],
        "pipeline-parallel-size": os.environ["PIPELINE_PARALLELISM"],
    }
)
