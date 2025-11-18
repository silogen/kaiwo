-- Transform audit logs to unified schema
-- Only processes create, update, delete operations
-- Input: Record with audit_json field (JSON string)
-- Output: Unified schema with object, time, msg, level, type, raw, meta

function parse_audit_logs(tag, timestamp, record)
    -- Only process audit logs
    if record["log_type"] ~= "audit" then
        return 2, timestamp, record  -- Pass through unchanged
    end

    -- Save original log line
    local raw_log = record["log"] or ""

    -- Get the audit_json string (extracted by parser filter)
    local audit_json = record["audit_json"]

    if not audit_json then
        -- No audit_json extracted, skip
        return 2, timestamp, record
    end

    -- Extract verb from JSON string using regex
    local verb = audit_json:match('"verb"%s*:%s*"([^"]+)"')

    if not verb then
        -- No verb found, skip
        return 2, timestamp, record
    end

    -- Filter: only process create, update, delete operations
    if verb ~= "create" and verb ~= "update" and verb ~= "delete" then
        -- Skip this audit log (not an interesting verb)
        return 1, timestamp, record  -- Return code 1 = drop record
    end

    -- Extract object reference fields from JSON string
    -- Need to find the objectRef section first, then extract fields from within it
    local obj_ref_section = audit_json:match('"objectRef"%s*:%s*(%b{})')

    local obj_api_version = ""
    local obj_resource = ""
    local obj_namespace = ""
    local obj_name = ""

    if obj_ref_section then
        obj_api_version = obj_ref_section:match('"apiVersion"%s*:%s*"([^"]+)"') or ""
        obj_resource = obj_ref_section:match('"resource"%s*:%s*"([^"]+)"') or ""
        obj_namespace = obj_ref_section:match('"namespace"%s*:%s*"([^"]+)"') or ""
        obj_name = obj_ref_section:match('"name"%s*:%s*"([^"]+)"') or ""
    end

    local object = {
        apiVersion = obj_api_version,
        kind = obj_resource,  -- Note: audit logs use "resource" not "kind"
        namespace = obj_namespace,
        name = obj_name
    }

    -- Extract time
    local time = audit_json:match('"requestReceivedTimestamp"%s*:%s*"([^"]*)"') or
                 audit_json:match('"stageTimestamp"%s*:%s*"([^"]*)"') or ""

    -- Construct message
    local msg = string.format("%s %s %s in namespace %s",
        verb,
        object.kind,
        object.name,
        object.namespace)

    -- Derive level from response status code
    local level = "info"
    local status_code_str = audit_json:match('"responseStatus"%s*:%s*{.-"code"%s*:%s*(%d+)')
    if status_code_str then
        local status_code = tonumber(status_code_str)
        if status_code >= 500 then
            level = "error"
        elseif status_code >= 400 then
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
