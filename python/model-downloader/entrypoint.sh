#!/bin/sh
set -eu

URL="${1:?Usage: $0 <hf://org/model or s3://bucket/path>}"
TARGET_DIR="${TARGET_DIR:-/cache}"


# Fetch expected size if not already set
if [ -z "${EXPECTED_SIZE_BYTES:-}" ]; then
    case "$URL" in
        hf://*)
        # Fetch expected size if not set
        if [ -z "${EXPECTED_SIZE_BYTES:-}" ]; then
            echo "Fetching model size from Hugging Face..."
            MODEL_PATH="${URL#hf://}"
            EXPECTED_SIZE_BYTES=$(python -c "
                from huggingface_hub import HfApi
                info = HfApi().model_info('$MODEL_PATH', files_metadata=True)
                print(sum(f.size or 0 for f in info.siblings))
                " 2>/dev/null || echo 0)
        fi
        ;;
        s3://*)
            # Get size from S3 (s3cmd du returns human-readable, need bytes)
            EXPECTED_SIZE_BYTES=$(s3cmd du "$URL" 2>/dev/null | awk '{print $1}' || echo 0)
            ;;
    esac
    export EXPECTED_SIZE_BYTES
fi

echo "Expected size: $EXPECTED_SIZE_BYTES bytes"

# Start progress monitor in background
if [ -f /progress_monitor.sh ]; then
    /progress_monitor.sh &
    echo "Started progress monitor (PID: $!)"
fi

### TESTING WHEN ENV VARS ARE SET ###
if [ -n "${AIM_DEBUG_CAUSE_HANG:-}" ]; then
    echo "AIM_DEBUG_CAUSE_HANG is set, causing hang"
    python -c "import time; time.sleep(1000000)"
    exit 1
fi

if [ -n "${AIM_DEBUG_CAUSE_FAILURE:-}" ]; then
    echo "AIM_DEBUG_CAUSE_FAILURE is set, causing failure"
    exit 1
fi
### END TESTING ###

case "$URL" in
    hf://*)
        export HF_HOME="$TARGET_DIR/.hf"
        mkdir -p "$HF_HOME"
        
        MODEL_PATH="${URL#hf://}"
        echo "Downloading from Hugging Face: $MODEL_PATH to $TARGET_DIR"
        hf download \
            --local-dir "$TARGET_DIR" \
            "$MODEL_PATH"
        echo "Verifying download..."
        hf cache verify \
            --local-dir "$TARGET_DIR" \
            --fail-on-missing-files \
            "$MODEL_PATH"
        echo "Download complete and verified"
        echo "Size of HF_HOME: $(du -sh "$HF_HOME")"
        rm -rf "$HF_HOME"
        ;;
    s3://*)
        echo "Syncing from S3: $URL to $TARGET_DIR"
        
        S3CMD_ARGS=""
        
        if [ -n "${S3_ACCESS_KEY:-}" ]; then
            S3CMD_ARGS="$S3CMD_ARGS --access_key=$S3_ACCESS_KEY"
        fi
        if [ -n "${S3_SECRET_KEY:-}" ]; then
            S3CMD_ARGS="$S3CMD_ARGS --secret_key=$S3_SECRET_KEY"
        fi
        if [ -n "${S3_ENDPOINT:-}" ]; then
            S3CMD_ARGS="$S3CMD_ARGS --host=$S3_ENDPOINT"
            S3CMD_ARGS="$S3CMD_ARGS --host-bucket=$S3_ENDPOINT/%(bucket)s"
        fi
        if [ "${S3_NO_SSL:-}" = "true" ]; then
            S3CMD_ARGS="$S3CMD_ARGS --no-ssl"
        fi
        if [ "${S3_SIGNATURE_V2:-}" = "true" ]; then
            S3CMD_ARGS="$S3CMD_ARGS --signature-v2"
        fi
        
        # shellcheck disable=SC2086
        s3cmd $S3CMD_ARGS sync --stop-on-error "$URL" "$TARGET_DIR/"
        echo "Sync complete"
        ;;
    *)
        echo "Error: Unknown protocol. URL must start with hf:// or s3://" >&2
        exit 1
        ;;
esac