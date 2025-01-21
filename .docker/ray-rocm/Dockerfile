FROM rocm/vllm-dev:20250104

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    wget && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* 

WORKDIR /

COPY requirements.txt .

RUN pip install -r requirements.txt --no-cache-dir

RUN pip install -U "ray[data,train,tune,serve,default] @ https://s3-us-west-2.amazonaws.com/ray-wheels/latest/ray-3.0.0.dev0-cp312-cp312-manylinux2014_x86_64.whl"

RUN pip install --force-reinstall \
    'https://github.com/bitsandbytes-foundation/bitsandbytes/releases/download/continuous-release_multi-backend-refactor/bitsandbytes-0.44.1.dev0-py3-none-manylinux_2_24_x86_64.whl' \
    --no-deps --no-cache-dir 

RUN pip uninstall -y triton && pip install -U triton 

RUN sudo sysctl kernel.numa_balancing=0

WORKDIR /workload

ENV VLLM_USE_TRITON_FLASH_ATTN=0
ENV TORCH_NCCL_HIGH_PRIORITY="1"
ENV LD_LIBRARY_PATH=/usr/lib64:$LD_LIBRARY_PATH