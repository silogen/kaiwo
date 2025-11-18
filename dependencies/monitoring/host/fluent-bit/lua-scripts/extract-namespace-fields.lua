-- Extract installer, run_id, run_attempt from namespace name
-- Pattern: test-{installer}-{run-id}-{run-attempt}
-- Example: test-ci-550e8400-e29b-41d4-a89f-446657440000-1
function extract_namespace_fields(tag, timestamp, record)
    local namespace = record["kubernetes"]["namespace_name"]

    if namespace then
        -- Pattern: test-{installer}-{run-id}-{run-attempt}
        local installer, run_id, run_attempt = string.match(namespace, "^test%-([^%-]+)%-(.+)%-(%d+)$")

        if installer and run_id and run_attempt then
            record["installer"] = installer
            record["run_id"] = run_id
            record["run_attempt"] = run_attempt
        end
    end

    return 2, timestamp, record
end