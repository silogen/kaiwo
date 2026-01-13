FROM python:3.14-alpine

RUN apk add --no-cache s3cmd procps kubectl \
    && pip install --no-cache-dir -U huggingface_hub

COPY entrypoint.sh /entrypoint.sh
COPY progress_monitor.sh /progress_monitor.sh
RUN chmod +x /entrypoint.sh /progress_monitor.sh

RUN mkdir /cache && chown 1000:1000 /cache


WORKDIR /cache
USER 1000

ENTRYPOINT ["/entrypoint.sh"]