#!/usr/bin/env bash

# Upload Chainsaw Test Results to PostgreSQL and Loki
# This script parses chainsaw-report.json and failures.json and uploads:
# 1. Test metadata to PostgreSQL for structured queries
# 2. Test execution logs to Loki for correlation with cluster logs
#
# Usage:
#   ./upload-test-results.sh [OPTIONS]
#
# Options:
#   -r, --report FILE          Path to chainsaw-report.json (default: test/chainsaw/chainsaw-report.json)
#   -f, --failures FILE        Path to failures.json (default: test/chainsaw/failures.json)
#   -i, --run-id ID           Unique run identifier (default: auto-generated)
#   -n, --namespace NS        vCluster namespace (default: unknown)
#   -g, --github-run ID       GitHub run ID (default: local)
#   -a, --github-attempt NUM  GitHub run attempt (default: 1)
#   --installer TYPE          Installer type: helm|kustomization|manual (default: manual)
#   --db-host HOST            PostgreSQL host (default: test-results-db-postgresql.test-observability...)
#   --db-port PORT            PostgreSQL port (default: 5432)
#   --db-name NAME            Database name (default: testresults)
#   --db-user USER            Database user (default: chainsaw)
#   --db-password PASS        Database password (default: chainsawtest)
#   --loki-url URL            Loki push endpoint (default: http://loki-0.loki-headless...)
#   --skip-loki               Skip pushing logs to Loki
#   --skip-db                 Skip inserting data to PostgreSQL
#   -h, --help                Show this help message
#
# Examples:
#   # Upload with custom report file
#   ./upload-test-results.sh --report /tmp/my-report.json
#
#   # Upload with custom run ID and namespace
#   ./upload-test-results.sh --run-id ci-123 --namespace kaiwo-test-ci-1-helm
#
#   # Upload to localhost PostgreSQL (via port-forward)
#   ./upload-test-results.sh --db-host localhost --db-port 5432
#
#   # Skip Loki upload (only PostgreSQL)
#   ./upload-test-results.sh --skip-loki

set -euo pipefail

# Show help
show_help() {
    sed -n '3,34p' "$0" | sed 's/^# //' | sed 's/^#//'
    exit 0
}

# Default configuration (can be overridden by environment or CLI args)
REPORT_FILE="${REPORT_FILE:-test/chainsaw/chainsaw-report.json}"
FAILURES_FILE="${FAILURES_FILE:-test/chainsaw/failures.json}"
DB_HOST="${DB_HOST:-test-results-db-postgresql.test-observability.svc.cluster.local}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-testresults}"
DB_USER="${DB_USER:-chainsaw}"
DB_PASSWORD="${DB_PASSWORD:-chainsawtest}"
LOKI_URL="${LOKI_URL:-http://loki-0.loki-headless.test-observability.svc.cluster.local:3100}"
SKIP_LOKI="${SKIP_LOKI:-false}"
SKIP_DB="${SKIP_DB:-false}"

# Test run metadata (from environment or defaults)
RUN_ID="${RUN_ID:-$(date +%s)-local}"
GITHUB_RUN_ID="${GITHUB_RUN_ID:-local}"
GITHUB_RUN_ATTEMPT="${GITHUB_RUN_ATTEMPT:-1}"
VCLUSTER_NAMESPACE="${VCLUSTER_NAMESPACE:-unknown}"
INSTALLER="${INSTALLER:-manual}"

# Parse command-line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -r|--report)
                REPORT_FILE="$2"
                shift 2
                ;;
            -f|--failures)
                FAILURES_FILE="$2"
                shift 2
                ;;
            -i|--run-id)
                RUN_ID="$2"
                shift 2
                ;;
            -n|--namespace)
                VCLUSTER_NAMESPACE="$2"
                shift 2
                ;;
            -g|--github-run)
                GITHUB_RUN_ID="$2"
                shift 2
                ;;
            -a|--github-attempt)
                GITHUB_RUN_ATTEMPT="$2"
                shift 2
                ;;
            --installer)
                INSTALLER="$2"
                shift 2
                ;;
            --db-host)
                DB_HOST="$2"
                shift 2
                ;;
            --db-port)
                DB_PORT="$2"
                shift 2
                ;;
            --db-name)
                DB_NAME="$2"
                shift 2
                ;;
            --db-user)
                DB_USER="$2"
                shift 2
                ;;
            --db-password)
                DB_PASSWORD="$2"
                shift 2
                ;;
            --loki-url)
                LOKI_URL="$2"
                shift 2
                ;;
            --skip-loki)
                SKIP_LOKI=true
                shift
                ;;
            --skip-db)
                SKIP_DB=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                echo "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Check dependencies
check_dependencies() {
    local missing=()
    for cmd in jq psql curl; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing required commands: ${missing[*]}"
        log_error "Please install: apt-get install jq postgresql-client curl"
        exit 1
    fi
}

# Check if report files exist
check_report_files() {
    if [ ! -f "$REPORT_FILE" ]; then
        log_error "Report file not found: $REPORT_FILE"
        exit 1
    fi

    if [ ! -f "$FAILURES_FILE" ]; then
        log_warn "Failures file not found: $FAILURES_FILE (continuing anyway)"
    fi
}

# Execute SQL query
exec_sql() {
    local query="$1"
    PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "$query"
}

# Insert test run record
insert_test_run() {
    log_info "Inserting test run: $RUN_ID"

    local start_time
    local end_time
    local total_tests
    local passed_tests
    local failed_tests

    start_time=$(jq -r '.startTime' "$REPORT_FILE")
    end_time=$(jq -r '.endTime' "$REPORT_FILE")
    total_tests=$(jq '.tests | length' "$REPORT_FILE")

    # Get pass/fail counts from failures.json if available
    if [ -f "$FAILURES_FILE" ]; then
        passed_tests=$(jq -r '.summary.passed // 0' "$FAILURES_FILE")
        failed_tests=$(jq -r '.summary.failed // 0' "$FAILURES_FILE")
    else
        # Fallback: count from report
        passed_tests=0
        failed_tests=0
    fi

    exec_sql "
        INSERT INTO test_runs (run_id, github_run_id, github_run_attempt, vcluster_namespace, installer, start_time, end_time, total_tests, passed_tests, failed_tests)
        VALUES ('$RUN_ID', '$GITHUB_RUN_ID', '$GITHUB_RUN_ATTEMPT', '$VCLUSTER_NAMESPACE', '$INSTALLER', '$start_time', '$end_time', $total_tests, $passed_tests, $failed_tests)
        ON CONFLICT (run_id) DO UPDATE SET
            end_time = EXCLUDED.end_time,
            total_tests = EXCLUDED.total_tests,
            passed_tests = EXCLUDED.passed_tests,
            failed_tests = EXCLUDED.failed_tests,
            report_uploaded_at = CURRENT_TIMESTAMP;
    "

    log_info "Test run inserted: $total_tests tests ($passed_tests passed, $failed_tests failed)"
}

# Get failure info for a test from failures.json
get_test_failure_info() {
    local test_name="$1"

    if [ ! -f "$FAILURES_FILE" ]; then
        echo "{}"
        return
    fi

    jq --arg name "$test_name" '.failures[] | select(.name == $name)' "$FAILURES_FILE"
}

# Determine test status
get_test_status() {
    local test_name="$1"
    local failure_info

    failure_info=$(get_test_failure_info "$test_name")

    if [ -n "$failure_info" ] && [ "$failure_info" != "{}" ]; then
        echo "failed"
    else
        echo "passed"
    fi
}

# Insert tests and their steps/operations
insert_tests() {
    log_info "Inserting test data..."

    local test_count
    test_count=$(jq '.tests | length' "$REPORT_FILE")

    for i in $(seq 0 $((test_count - 1))); do
        local test_name namespace base_path start_time end_time concurrent status duration test_id

        test_name=$(jq -r ".tests[$i].name" "$REPORT_FILE")
        namespace=$(jq -r ".tests[$i].namespace // \"\"" "$REPORT_FILE")
        base_path=$(jq -r ".tests[$i].basePath // \"\"" "$REPORT_FILE")
        start_time=$(jq -r ".tests[$i].startTime" "$REPORT_FILE")
        end_time=$(jq -r ".tests[$i].endTime" "$REPORT_FILE")
        concurrent=$(jq -r ".tests[$i].concurrent // false" "$REPORT_FILE")
        status=$(get_test_status "$test_name")

        # Calculate duration
        if [ "$end_time" != "null" ] && [ "$start_time" != "null" ]; then
            duration=$(jq -n --arg start "$start_time" --arg end "$end_time" \
                '((($end | fromdateiso8601) - ($start | fromdateiso8601)) * 100 | round) / 100')
        else
            duration="null"
        fi

        # Insert test
        test_id=$(exec_sql "
            INSERT INTO tests (run_id, test_name, base_path, namespace, status, start_time, end_time, duration_seconds, concurrent)
            VALUES ('$RUN_ID', '$test_name', '$base_path', '$namespace', '$status', '$start_time', '$end_time', $duration, $concurrent)
            RETURNING test_id;
        " | tr -d ' ')

        log_info "  [$((i+1))/$test_count] $test_name ($status)"

        # Insert steps and operations
        insert_steps "$i" "$test_id" "$test_name" "$status"
    done
}

# Insert steps for a test
insert_steps() {
    local test_index="$1"
    local test_id="$2"
    local test_name="$3"
    local test_status="$4"

    local step_count
    step_count=$(jq ".tests[$test_index].steps | length" "$REPORT_FILE")

    # Get failure info if test failed
    local failure_info
    failure_info=$(get_test_failure_info "$test_name")

    for j in $(seq 0 $((step_count - 1))); do
        local step_name start_time end_time duration step_id step_status

        step_name=$(jq -r ".tests[$test_index].steps[$j].name" "$REPORT_FILE")
        start_time=$(jq -r ".tests[$test_index].steps[$j].startTime" "$REPORT_FILE")
        end_time=$(jq -r ".tests[$test_index].steps[$j].endTime" "$REPORT_FILE")

        # Calculate duration
        if [ "$end_time" != "null" ] && [ "$start_time" != "null" ]; then
            duration=$(jq -n --arg start "$start_time" --arg end "$end_time" \
                '((($end | fromdateiso8601) - ($start | fromdateiso8601)) * 100 | round) / 100')
        else
            duration="null"
        fi

        # Check if this step failed
        step_status="passed"
        if [ "$test_status" = "failed" ] && [ -n "$failure_info" ] && [ "$failure_info" != "{}" ]; then
            if echo "$failure_info" | jq -e --arg name "$step_name" '.failedSteps[] | select(.stepName == $name)' > /dev/null 2>&1; then
                step_status="failed"
            fi
        fi

        # Insert step
        step_id=$(exec_sql "
            INSERT INTO test_steps (test_id, step_name, step_index, status, start_time, end_time, duration_seconds)
            VALUES ($test_id, '$step_name', $j, '$step_status', '$start_time', '$end_time', $duration)
            RETURNING step_id;
        " | tr -d ' ')

        # Insert operations
        insert_operations "$test_index" "$j" "$step_id" "$test_name" "$step_name" "$step_status"
    done
}

# Insert operations for a step
insert_operations() {
    local test_index="$1"
    local step_index="$2"
    local step_id="$3"
    local test_name="$4"
    local step_name="$5"
    local step_status="$6"

    local op_count
    op_count=$(jq ".tests[$test_index].steps[$step_index].operations | length" "$REPORT_FILE")

    # Get failure info
    local failure_info
    failure_info=$(get_test_failure_info "$test_name")

    for k in $(seq 0 $((op_count - 1))); do
        local op_name op_type start_time end_time duration op_status error_message

        op_name=$(jq -r ".tests[$test_index].steps[$step_index].operations[$k].name" "$REPORT_FILE")
        op_type=$(jq -r ".tests[$test_index].steps[$step_index].operations[$k].type" "$REPORT_FILE")
        start_time=$(jq -r ".tests[$test_index].steps[$step_index].operations[$k].startTime" "$REPORT_FILE")
        end_time=$(jq -r ".tests[$test_index].steps[$step_index].operations[$k].endTime" "$REPORT_FILE")

        # Calculate duration
        if [ "$end_time" != "null" ] && [ "$start_time" != "null" ]; then
            duration=$(jq -n --arg start "$start_time" --arg end "$end_time" \
                '((($end | fromdateiso8601) - ($start | fromdateiso8601)) * 100 | round) / 100')
        else
            duration="null"
        fi

        # Check if this operation failed
        op_status="passed"
        error_message=""

        if [ "$step_status" = "failed" ] && [ -n "$failure_info" ] && [ "$failure_info" != "{}" ]; then
            local op_failure
            op_failure=$(echo "$failure_info" | jq -r --arg step "$step_name" --arg op "$op_name" \
                '.failedSteps[] | select(.stepName == $step) | .failedOperations[] | select(.operationName == $op) | .error // ""' 2>/dev/null || echo "")

            if [ -n "$op_failure" ]; then
                op_status="failed"
                error_message=$(echo "$op_failure" | sed "s/'/''/g")  # Escape single quotes for SQL
            fi
        fi

        # Insert operation
        exec_sql "
            INSERT INTO test_operations (step_id, operation_name, operation_index, operation_type, status, start_time, end_time, duration_seconds, error_message)
            VALUES ($step_id, '$op_name', $k, '$op_type', '$op_status', '$start_time', '$end_time', $duration, $([ -n "$error_message" ] && echo "'$error_message'" || echo "null"));
        " > /dev/null
    done
}

# Push logs to Loki
push_to_loki() {
    log_info "Pushing test logs to Loki..."

    local test_count
    test_count=$(jq '.tests | length' "$REPORT_FILE")

    # Create a streams array for Loki push API
    local streams_json='{"streams":[]}'

    for i in $(seq 0 $((test_count - 1))); do
        local test_name namespace base_path status start_time

        test_name=$(jq -r ".tests[$i].name" "$REPORT_FILE")
        namespace=$(jq -r ".tests[$i].namespace // \"unknown\"" "$REPORT_FILE")
        base_path=$(jq -r ".tests[$i].basePath // \"\"" "$REPORT_FILE")
        status=$(get_test_status "$test_name")
        start_time=$(jq -r ".tests[$i].startTime" "$REPORT_FILE")

        # Convert timestamp to nanoseconds
        local timestamp_ns
        timestamp_ns=$(jq -n --arg ts "$start_time" '(($ts | fromdateiso8601) * 1000000000 | floor)')

        # Get step count
        local step_count
        step_count=$(jq ".tests[$i].steps | length" "$REPORT_FILE")

        # Push logs for each step and operation
        for j in $(seq 0 $((step_count - 1))); do
            local step_name step_start op_count

            step_name=$(jq -r ".tests[$i].steps[$j].name" "$REPORT_FILE")
            step_start=$(jq -r ".tests[$i].steps[$j].startTime" "$REPORT_FILE")
            op_count=$(jq ".tests[$i].steps[$j].operations | length" "$REPORT_FILE")

            for k in $(seq 0 $((op_count - 1))); do
                local op_name op_type op_start op_end op_status error_msg level

                op_name=$(jq -r ".tests[$i].steps[$j].operations[$k].name" "$REPORT_FILE")
                op_type=$(jq -r ".tests[$i].steps[$j].operations[$k].type" "$REPORT_FILE")
                op_start=$(jq -r ".tests[$i].steps[$j].operations[$k].startTime" "$REPORT_FILE")
                op_end=$(jq -r ".tests[$i].steps[$j].operations[$k].endTime" "$REPORT_FILE")

                # Check for error
                error_msg=$(jq -r ".tests[$i].steps[$j].operations[$k].failure.error // \"\"" "$REPORT_FILE")

                if [ -n "$error_msg" ]; then
                    op_status="failed"
                    level="error"
                else
                    op_status="passed"
                    level="info"
                fi

                # Create log message
                local log_msg
                if [ -n "$error_msg" ]; then
                    log_msg="Test: $test_name | Step: $step_name | Operation: $op_name ($op_type) | Status: FAILED | Error: $error_msg"
                else
                    log_msg="Test: $test_name | Step: $step_name | Operation: $op_name ($op_type) | Status: PASSED"
                fi

                # Escape JSON
                log_msg=$(echo "$log_msg" | jq -Rs .)

                # Convert timestamp
                local op_timestamp_ns
                op_timestamp_ns=$(jq -n --arg ts "$op_start" '(($ts | fromdateiso8601) * 1000000000 | floor)')

                # Create stream entry (we'll batch these)
                local stream_entry
                stream_entry=$(cat <<EOF
{
  "stream": {
    "log_type": "chainsaw_test",
    "vcluster_namespace": "$VCLUSTER_NAMESPACE",
    "github_run_id": "$GITHUB_RUN_ID",
    "github_run_attempt": "$GITHUB_RUN_ATTEMPT",
    "installer": "$INSTALLER",
    "test_name": "$test_name",
    "namespace": "$namespace",
    "test_status": "$status",
    "step_name": "$step_name",
    "operation_name": "$op_name",
    "operation_type": "$op_type",
    "operation_status": "$op_status",
    "level": "$level",
    "kind": "ChainsawTest",
    "object_kind": "ChainsawTest"
  },
  "values": [
    ["$op_timestamp_ns", $log_msg]
  ]
}
EOF
)

                # Add to streams array
                streams_json=$(echo "$streams_json" | jq --argjson entry "$stream_entry" '.streams += [$entry]')
            done
        done
    done

    # Push to Loki in batches (Loki has limits on request size)
    # For now, push everything at once
    if [ "$(echo "$streams_json" | jq '.streams | length')" -gt 0 ]; then
        local response
        response=$(curl -s -w "\n%{http_code}" -X POST "$LOKI_URL/loki/api/v1/push" \
            -H "Content-Type: application/json" \
            -d "$streams_json")

        local http_code
        http_code=$(echo "$response" | tail -n1)

        if [ "$http_code" = "204" ] || [ "$http_code" = "200" ]; then
            log_info "Successfully pushed $(echo "$streams_json" | jq '.streams | length') log entries to Loki"
        else
            log_error "Failed to push to Loki (HTTP $http_code)"
            echo "$response" | head -n-1
        fi
    fi
}

# Main execution
main() {
    # Parse command-line arguments first
    parse_args "$@"

    log_info "Starting Chainsaw test results upload"
    log_info "Run ID: $RUN_ID"
    log_info "GitHub Run: $GITHUB_RUN_ID (attempt $GITHUB_RUN_ATTEMPT)"
    log_info "vCluster Namespace: $VCLUSTER_NAMESPACE"
    log_info "Report File: $REPORT_FILE"

    if [ "$SKIP_DB" = "true" ] && [ "$SKIP_LOKI" = "true" ]; then
        log_error "Both --skip-db and --skip-loki specified. Nothing to do!"
        exit 1
    fi

    check_dependencies
    check_report_files

    if [ "$SKIP_DB" = "true" ]; then
        log_warn "Skipping PostgreSQL upload (--skip-db specified)"
    else
        insert_test_run
        insert_tests
    fi

    if [ "$SKIP_LOKI" = "true" ]; then
        log_warn "Skipping Loki upload (--skip-loki specified)"
    else
        push_to_loki
    fi

    log_info "Upload complete!"

    if [ "$SKIP_DB" != "true" ]; then
        log_info "Query test results: psql -h $DB_HOST -U $DB_USER -d $DB_NAME"
    fi

    if [ "$SKIP_LOKI" != "true" ]; then
        log_info "View in Grafana: {log_type=\"chainsaw_test\", vcluster_namespace=\"$VCLUSTER_NAMESPACE\"}"
    fi
}

main "$@"
