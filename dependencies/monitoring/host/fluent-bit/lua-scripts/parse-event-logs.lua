-- Transform event logs to unified schema
-- Handles both K8s Event objects and event-exporter error logs
-- Input: Record with log field containing JSON string
-- Output: Unified schema with object, time, msg, level, type, raw, meta

function parse_event_logs(tag, timestamp, record)
    -- Only process event logs
    if record["log_type"] ~= "event" then
        return 2, timestamp, record  -- Pass through unchanged
    end

    -- Save original log line
    local raw_log = record["log"] or ""

    -- Extract JSON string from CRI-formatted log
    -- Format: "TIMESTAMP STREAM TAG {JSON}"
    local json_str = raw_log:match('{.+}')

    if not json_str then
        -- No JSON found, keep original
        return 2, timestamp, record
    end

    -- Since we can't use cjson, we'll parse fields using string matching
    -- This is a workaround for basic JSON field extraction

    local object, time, msg, level

    -- Check for K8s Event format (has involvedObject field)
    if json_str:match('"involvedObject"%s*:') then
        -- K8s Event format
        -- Extract involvedObject fields
        local involved_api_version = json_str:match('"involvedObject"%s*:%s*{.-"apiVersion"%s*:%s*"([^"]+)"') or ""
        local involved_kind = json_str:match('"involvedObject"%s*:%s*{.-"kind"%s*:%s*"([^"]+)"') or ""
        local involved_namespace = json_str:match('"involvedObject"%s*:%s*{.-"namespace"%s*:%s*"([^"]+)"') or ""
        local involved_name = json_str:match('"involvedObject"%s*:%s*{.-"name"%s*:%s*"([^"]+)"') or ""

        object = {
            apiVersion = involved_api_version,
            kind = involved_kind,
            namespace = involved_namespace,
            name = involved_name
        }

        -- Extract other fields
        time = json_str:match('"lastTimestamp"%s*:%s*"([^"]+)"') or json_str:match('"firstTimestamp"%s*:%s*"([^"]+)"') or ""
        msg = json_str:match('"message"%s*:%s*"([^"]+)"') or ""

        -- Map event type to level
        local event_type = json_str:match('"type"%s*:%s*"([^"]+)"')
        if event_type == "Warning" then
            level = "warning"
        else
            level = "info"
        end

    -- Check for error log format (has level and message fields)
    elseif json_str:match('"level"%s*:') and json_str:match('"message"%s*:') then
        -- Event-exporter error log format
        object = {
            apiVersion = "",
            kind = "",
            namespace = "",
            name = ""
        }

        time = json_str:match('"time"%s*:%s*"([^"]+)"') or ""
        msg = json_str:match('"message"%s*:%s*"([^"]+)"') or ""
        level = json_str:match('"level"%s*:%s*"([^"]+)"') or "info"

        -- Append error details if present
        local error_details = json_str:match('"error"%s*:%s*"([^"]+)"')
        if error_details then
            msg = msg .. ": " .. error_details
        end

    else
        -- Unknown format, keep original
        return 2, timestamp, record
    end

    -- Build unified record
    local unified = {
        object = object,
        time = time,
        msg = msg,
        level = level,
        type = "event",
        details = {},  -- Will populate later
        raw = raw_log,
        meta = {
            installer = record["installer"],
            run_id = record["run_id"],
            run_attempt = record["run_attempt"],
            log_type = record["log_type"]
        }
    }

    return 2, timestamp, unified
end
