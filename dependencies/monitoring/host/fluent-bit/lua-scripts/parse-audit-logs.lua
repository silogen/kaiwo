-- Transform audit logs to unified schema
-- Only processes create, update, delete operations
-- Input: Record with parsed audit fields from Parser filter
-- Output: Unified schema with object, time, msg, level, type, raw, meta

function parse_audit_logs(tag, timestamp, record)
    -- Only process audit logs
    if record["log_type"] ~= "audit" then
        return 2, timestamp, record  -- Pass through unchanged
    end

    -- Save original log line
    local raw_log = record["log"] or ""

    -- Check if audit fields were parsed (from Parser filter)
    -- Parser filter adds fields directly to record
    local verb = record["verb"]

    if not verb then
        -- Audit JSON wasn't parsed, skip
        return 2, timestamp, record
    end

    -- Filter: only process create, update, delete operations
    if verb ~= "create" and verb ~= "update" and verb ~= "delete" then
        -- Skip this audit log (not an interesting verb)
        return 1, timestamp, record  -- Return code 1 = drop record
    end

    -- Extract object reference from parsed fields
    -- Parser flattens nested objects, so objectRef.namespace becomes objectRef_namespace
    local object = {
        apiVersion = record["objectRef_apiVersion"] or record["objectRef.apiVersion"] or "",
        kind = record["objectRef_resource"] or record["objectRef.resource"] or "",  -- audit uses "resource"
        namespace = record["objectRef_namespace"] or record["objectRef.namespace"] or "",
        name = record["objectRef_name"] or record["objectRef.name"] or ""
    }

    -- Extract time
    local time = record["requestReceivedTimestamp"] or record["stageTimestamp"] or ""

    -- Construct message
    local msg = string.format("%s %s %s in namespace %s",
        verb,
        object.kind,
        object.name,
        object.namespace)

    -- Derive level from response status code
    local level = "info"
    local status_code = record["responseStatus_code"] or record["responseStatus.code"]
    if status_code then
        status_code = tonumber(status_code)
        if status_code and status_code >= 500 then
            level = "error"
        elseif status_code and status_code >= 400 then
            level = "warning"
        else
            level = "info"
        end
    end

    -- Build unified record
    local unified = {
        object = object,
        time = time,
        msg = msg,
        level = level,
        type = "audit",
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
