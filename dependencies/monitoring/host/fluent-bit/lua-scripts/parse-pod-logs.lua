-- Transform pod logs to unified schema
-- Attempts JSON parse and detects controller logs
-- Input: Pod log line (plain text or JSON)
-- Output: Unified schema with object, time, msg, level, type, raw, meta

function parse_pod_logs(tag, timestamp, record)
    -- Only process pod logs
    if record["log_type"] ~= "pod" then
        return 2, timestamp, record  -- Pass through unchanged
    end

    -- Save original log line
    local raw_log = record["log"] or ""

    -- Extract log content from CRI-formatted log
    -- Format: "TIMESTAMP STREAM TAG CONTENT"
    local log_content = raw_log:match("^%S+%s+%S+%s+%S+%s+(.*)$")
    if not log_content then
        -- Fallback: use entire log if pattern doesn't match
        log_content = raw_log
    end

    local object, time, msg, level

    -- Try to detect JSON by looking for opening brace
    local json_str = log_content:match('^%s*({.+})%s*$')

    if json_str then
        -- Looks like JSON, try to parse fields using string matching

        -- Check for controller log indicators: namespace, name, kind fields
        local has_namespace = json_str:match('"namespace"%s*:')
        local has_name = json_str:match('"name"%s*:')
        local has_kind = json_str:match('"kind"%s*:')

        if has_namespace and has_name and has_kind then
            -- Controller log format
            local api_version = json_str:match('"apiVersion"%s*:%s*"([^"]+)"') or json_str:match('"apiversion"%s*:%s*"([^"]+)"') or ""
            local kind = json_str:match('"kind"%s*:%s*"([^"]+)"') or ""
            local namespace = json_str:match('"namespace"%s*:%s*"([^"]+)"') or ""
            local name = json_str:match('"name"%s*:%s*"([^"]+)"') or ""

            object = {
                apiVersion = api_version,
                kind = kind,
                namespace = namespace,
                name = name
            }
        else
            -- JSON but not controller log
            object = {
                apiVersion = "",
                kind = "",
                namespace = "",
                name = ""
            }
        end

        -- Extract msg field or use entire JSON as message
        msg = json_str:match('"msg"%s*:%s*"([^"]+)"') or json_str:match('"message"%s*:%s*"([^"]+)"') or json_str

        -- Extract level
        level = json_str:match('"level"%s*:%s*"([^"]+)"') or "info"

        -- Extract time if present
        time = json_str:match('"time"%s*:%s*"([^"]+)"') or json_str:match('"timestamp"%s*:%s*"([^"]+)"') or json_str:match('"ts"%s*:%s*"([^"]+)"') or ""

    else
        -- Not JSON, treat as plain text

        object = {
            apiVersion = "",
            kind = "",
            namespace = "",
            name = ""
        }

        msg = log_content
        level = "info"
        time = ""
    end

    -- Build unified record
    local unified = {
        object = object,
        time = time,
        msg = msg,
        level = level,
        type = "pod",
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
