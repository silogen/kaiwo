FROM python:3.12-slim

RUN apt update && apt install -y --no-install-recommends wget && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir ray[default]