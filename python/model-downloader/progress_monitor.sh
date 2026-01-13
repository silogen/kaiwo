#!/bin/sh
# Progress monitor for model downloads
# Outputs JSON progress logs and kills stalled downloads

# Handle SIGTERM gracefully - kubelet sends this when main container terminates
terminated=false
trap 'terminated=true' TERM

# Output a JSON log message
# Usage: log_json <type> [key=value ...]
log_json() {
    type=$1
    shift
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    json="{\"timestamp\":\"$timestamp\",\"type\":\"$type\""
    for kv in "$@"; do
        key="${kv%%=*}"
        value="${kv#*=}"
        # Check if value is numeric
        case "$value" in
            ''|*[!0-9]*) json="$json,\"$key\":\"$value\"" ;;  # string
            *) json="$json,\"$key\":$value" ;;                 # number
        esac
    done
    echo "$json}" >&2
}

SA_TOKEN="/var/run/secrets/kubernetes.io/serviceaccount/token"
can_update_status=false
if [ -f "$SA_TOKEN" ] && [ -n "${CACHE_NAME:-}" ] && [ -n "${CACHE_NAMESPACE:-}" ]; then
    can_update_status=true
fi

update_status() {
    if [ "$can_update_status" = "true" ]; then
        percent=$1
        kubectl patch aimmodelcache "$CACHE_NAME" -n "$CACHE_NAMESPACE" \
            --type=merge --subresource=status \
            -p "{\"status\":{\"downloadProgress\":$percent}}" 2>/dev/null || true
    fi
}

# Kill the download process (huggingface-cli or s3cmd)
kill_downloader() {
    # Kill python processes
    pkill -9 -f "python" 2>/dev/null || true
    # Kill s3cmd
    pkill -9 -f "s3cmd" 2>/dev/null || true
    
}   

expected_size=${EXPECTED_SIZE_BYTES:-0}
mount_path=${MOUNT_PATH:-/cache}
log_interval=${PROGRESS_INTERVAL:-5} # 5 seconds default
stall_timeout=${STALL_TIMEOUT:-60}  # 1 minutes default

log_json "start" "expectedBytes=$expected_size" "intervalSeconds=$log_interval" "stallTimeoutSeconds=$stall_timeout"

last_size=0
last_change_time=$(date +%s)

while true; do
    # Check if we received SIGTERM (main container terminated)
    if [ "$terminated" = "true" ]; then
        current_size=$(du -sb "$mount_path" 2>/dev/null | cut -f1 || echo 0)
        log_json "terminated" "currentBytes=$current_size" "expectedBytes=$expected_size"
        exit 0
    fi

    current_size=$(du -sb "$mount_path" 2>/dev/null | cut -f1 || echo 0)
    now=$(date +%s)

    # Track progress for stall detection
    if [ "$current_size" -gt "$last_size" ]; then
        last_size=$current_size
        last_change_time=$now
    fi

    # Check for stall (no progress for stall_timeout seconds)
    stall_duration=$((now - last_change_time))
    if [ "$stall_duration" -ge "$stall_timeout" ]; then
        log_json "stall" "currentBytes=$current_size" "stallDurationSeconds=$stall_duration"
        kill_downloader
        exit 1
    fi

    if [ "$expected_size" -gt 0 ] && [ "$current_size" -gt 0 ]; then
        percent=$((current_size * 100 / expected_size))
        # Cap at 100% (during download, temp files may exceed expected size)
        [ "$percent" -gt 100 ] && percent=100
        log_json "progress" "percent=$percent" "currentBytes=$current_size" "expectedBytes=$expected_size"
    elif [ "$current_size" -gt 0 ]; then
        log_json "progress" "currentBytes=$current_size" "message=Expected size unknown"
    else
        log_json "waiting" "message=Waiting for download to start"
    fi

    update_status "$percent"

    # Use a loop with short sleeps so we can check for SIGTERM more frequently
    i=0
    while [ $i -lt $log_interval ] && [ "$terminated" = "false" ]; do
        sleep 1
        i=$((i + 1))
    done
done

