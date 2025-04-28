FROM rocm/vllm-dev:nightly_main_20250427

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    wget && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* 

WORKDIR /

RUN sudo useradd -m -u 1000 -s /bin/bash kaiwo && \
    sudo usermod -a -G render,video kaiwo && \
    chown -R kaiwo:kaiwo /home/kaiwo && \
    chmod 700 /home/kaiwo && \
    mkdir -p /workload && \
    chown -R kaiwo:kaiwo /workload && \
    chmod 700 /workload


COPY requirements.txt .

RUN pip install -r requirements.txt --no-cache-dir

RUN pip install -U ray[data,train,tune,serve,default] 

RUN pip install --force-reinstall \
    'https://github.com/bitsandbytes-foundation/bitsandbytes/releases/download/continuous-release_multi-backend-refactor/bitsandbytes-0.44.1.dev0-py3-none-manylinux_2_24_x86_64.whl' \
    --no-deps --no-cache-dir 


RUN pip install --no-deps jsonlines loralib optimum transformers_stream_generator && \
    git clone https://github.com/OpenRLHF/OpenRLHF.git && cd OpenRLHF && \
    python setup.py bdist_wheel && pip install --no-deps dist/*.whl

USER kaiwo

WORKDIR /workload

ENV VLLM_USE_TRITON_FLASH_ATTN=0
ENV RAY_EXPERIMENTAL_NOSET_ROCR_VISIBLE_DEVICES=0
ENV TORCH_NCCL_HIGH_PRIORITY="1"
# Use only when absolutely necessary (may cause performance degradation)
ENV NCCL_DISABLE_P2P=0
ENV LD_LIBRARY_PATH=/usr/lib64:$LD_LIBRARY_PATH
