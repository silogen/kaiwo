-- Classify logs into three types: audit, event, or pod
-- Uses pod labels for classification
function classify_log_type(tag, timestamp, record)
    local kubernetes = record["kubernetes"]
    if not kubernetes then
        record["log_type"] = "pod"
        return 2, timestamp, record
    end

    local labels = kubernetes["labels"]
    local app_label = labels and labels["app"] or nil

    -- Type 1: Audit logs (vCluster pods have app=vcluster label)
    if app_label == "vcluster" then
        record["log_type"] = "audit"
        return 2, timestamp, record
    end

    -- Type 2: Kubernetes Events (event-exporter pods have app=event-exporter label)
    if app_label == "event-exporter" then
        record["log_type"] = "event"
        return 2, timestamp, record
    end

    -- Type 3: Pod logs (everything else)
    record["log_type"] = "pod"

    return 2, timestamp, record
end