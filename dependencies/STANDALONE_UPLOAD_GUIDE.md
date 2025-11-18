# Standalone Test Results Upload Guide

This guide explains how to use the `upload-test-results.sh` script independently from the test runner.

## When to Use Standalone Upload

- Tests were run directly with `chainsaw` (not via `run-tests.sh`)
- Re-uploading results after fixing database/Loki issues
- Uploading results from a different machine/environment
- Custom test execution workflows
- Batch processing of multiple test reports

## Prerequisites

- `jq` - JSON processor
- `psql` - PostgreSQL client
- `curl` - HTTP client
- Test report files: `chainsaw-report.json` (and optionally `failures.json`)

## Basic Usage

### Simple Upload

Upload results using default settings:

```bash
cd /path/to/ai-workload-orchestrator

# Upload with defaults (auto-generated run ID)
./dependencies/upload-test-results.sh
```

### Upload with Custom Run ID

```bash
./dependencies/upload-test-results.sh \
  --run-id "my-test-run-123" \
  --namespace "kaiwo-test-local-1-helm"
```

### Upload from Custom Location

```bash
./dependencies/upload-test-results.sh \
  --report /tmp/chainsaw-reports/report-20250111.json \
  --failures /tmp/chainsaw-reports/failures-20250111.json \
  --run-id "test-20250111"
```

## Common Scenarios

### Scenario 1: Re-upload After Database Failure

Your tests completed, but database upload failed:

```bash
# Database was down during test execution
# Now it's up, re-upload the same results

./dependencies/upload-test-results.sh \
  --run-id "original-run-id-123" \
  --namespace "kaiwo-test-local-1-helm" \
  --report test/chainsaw/chainsaw-report.json
```

**Note:** Using the same `--run-id` will update the existing record (upsert).

### Scenario 2: Upload via Port-Forward

Database is only accessible via kubectl port-forward:

```bash
# In terminal 1: Port-forward PostgreSQL
kubectl port-forward -n test-observability svc/test-results-db-postgresql 5432:5432

# In terminal 2: Upload to localhost
./dependencies/upload-test-results.sh \
  --db-host localhost \
  --db-port 5432 \
  --run-id "local-test-123"
```

### Scenario 3: Upload Only to PostgreSQL (Skip Loki)

```bash
./dependencies/upload-test-results.sh \
  --skip-loki \
  --run-id "db-only-test"
```

**Use case:** Loki is down/unavailable, but you want test metadata in the database.

### Scenario 4: Upload Only to Loki (Skip PostgreSQL)

```bash
./dependencies/upload-test-results.sh \
  --skip-db \
  --run-id "loki-only-test"
```

**Use case:** Want test logs searchable in Loki, but don't need structured database queries.

### Scenario 5: Batch Upload Multiple Reports

```bash
#!/bin/bash
# Upload multiple test reports from CI artifacts

for report in /tmp/test-reports/*.json; do
    # Extract run ID from filename
    run_id=$(basename "$report" .json)

    echo "Uploading $run_id..."
    ./dependencies/upload-test-results.sh \
        --report "$report" \
        --run-id "$run_id" \
        --namespace "kaiwo-test-ci-batch"
done
```

### Scenario 6: CI Pipeline Integration

```yaml
# .github/workflows/test.yml
- name: Run Tests
  run: |
    chainsaw test --report-format JSON --report-path ./reports

- name: Upload Results (separate step)
  if: always()  # Upload even if tests failed
  env:
    RUN_ID: ${{ github.run_id }}-${{ github.run_attempt }}
    GITHUB_RUN_ID: ${{ github.run_id }}
    GITHUB_RUN_ATTEMPT: ${{ github.run_attempt }}
    VCLUSTER_NAMESPACE: ${{ env.VCLUSTER_NAMESPACE }}
  run: |
    ./dependencies/upload-test-results.sh \
      --report ./reports/chainsaw-report.json \
      --run-id "$RUN_ID" \
      --namespace "$VCLUSTER_NAMESPACE" \
      --github-run "$GITHUB_RUN_ID" \
      --github-attempt "$GITHUB_RUN_ATTEMPT" \
      --installer helm
```

## All Command-Line Options

```bash
./dependencies/upload-test-results.sh [OPTIONS]

File Options:
  -r, --report FILE          Path to chainsaw-report.json
                             Default: test/chainsaw/chainsaw-report.json

  -f, --failures FILE        Path to failures.json (optional)
                             Default: test/chainsaw/failures.json

Test Metadata:
  -i, --run-id ID           Unique run identifier
                             Default: auto-generated (timestamp-local)

  -n, --namespace NS        vCluster namespace
                             Default: unknown

  -g, --github-run ID       GitHub run ID
                             Default: local

  -a, --github-attempt NUM  GitHub run attempt number
                             Default: 1

  --installer TYPE          Installation method: helm|kustomization|manual
                             Default: manual

Database Options:
  --db-host HOST            PostgreSQL hostname
                             Default: test-results-db-postgresql.test-observability...

  --db-port PORT            PostgreSQL port
                             Default: 5432

  --db-name NAME            Database name
                             Default: testresults

  --db-user USER            Database username
                             Default: chainsaw

  --db-password PASS        Database password
                             Default: chainsawtest

Loki Options:
  --loki-url URL            Loki push endpoint
                             Default: http://loki-0.loki-headless.test-observability...

Control Options:
  --skip-loki               Don't push logs to Loki (only PostgreSQL)
  --skip-db                 Don't insert to PostgreSQL (only Loki)
  -h, --help                Show help message and exit
```

## Environment Variables

All options can also be set via environment variables:

```bash
export REPORT_FILE="/path/to/chainsaw-report.json"
export FAILURES_FILE="/path/to/failures.json"
export RUN_ID="my-test-run"
export GITHUB_RUN_ID="12345"
export GITHUB_RUN_ATTEMPT="2"
export VCLUSTER_NAMESPACE="kaiwo-test-ci-1-helm"
export INSTALLER="helm"
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_NAME="testresults"
export DB_USER="chainsaw"
export DB_PASSWORD="chainsawtest"
export LOKI_URL="http://loki:3100"
export SKIP_LOKI="false"
export SKIP_DB="false"

./dependencies/upload-test-results.sh
```

**Priority:** CLI arguments > Environment variables > Defaults

## Troubleshooting

### Error: "Report file not found"

```
[ERROR] Report file not found: test/chainsaw/chainsaw-report.json
```

**Solution:** Specify the correct path:
```bash
./dependencies/upload-test-results.sh --report /actual/path/to/report.json
```

### Error: "Missing required commands: psql"

```
[ERROR] Missing required commands: psql
[ERROR] Please install: apt-get install jq postgresql-client curl
```

**Solution:** Install dependencies:
```bash
# Ubuntu/Debian
sudo apt-get install jq postgresql-client curl

# macOS
brew install jq postgresql curl
```

### Error: "connection refused"

```
psql: error: connection to server at "test-results-db-postgresql..." failed: Connection refused
```

**Solutions:**

1. **Port-forward to PostgreSQL:**
   ```bash
   kubectl port-forward -n test-observability svc/test-results-db-postgresql 5432:5432
   ./dependencies/upload-test-results.sh --db-host localhost
   ```

2. **Check PostgreSQL is running:**
   ```bash
   kubectl get pods -n test-observability | grep postgresql
   ```

3. **Use in-cluster execution:**
   ```bash
   # Run upload from inside a pod
   kubectl run -it --rm upload-pod --image=postgres:15 -- bash
   # Inside pod:
   apt-get update && apt-get install -y jq curl
   ./dependencies/upload-test-results.sh
   ```

### Error: "Failed to push to Loki (HTTP 400)"

**Solution:** Check Loki is accepting writes:
```bash
# Test Loki endpoint
curl http://loki-0.loki-headless.test-observability.svc.cluster.local:3100/ready

# Skip Loki if it's down
./dependencies/upload-test-results.sh --skip-loki
```

### Warning: "Failures file not found (continuing anyway)"

```
[WARN] Failures file not found: test/chainsaw/failures.json (continuing anyway)
```

**This is OK!** The failures file is optional. Upload will continue with just the main report.

To generate failures file:
```bash
# Use run-tests.sh which auto-generates it
./test/chainsaw/run-tests.sh

# Or manually create it from report
jq '{summary: {total: (.tests | length), ...}, failures: [...]}' \
  chainsaw-report.json > failures.json
```

## Advanced Usage

### Custom Failure Processing

Generate custom failures.json before upload:

```bash
#!/bin/bash
# Extract only specific error types

jq '{
  summary: {
    total: (.tests | length),
    passed: [.tests[] | select(.status == "passed")] | length,
    failed: [.tests[] | select(.status == "failed")] | length
  },
  failures: [
    .tests[]
    | select(.status == "failed")
    | select(.error | contains("timeout"))  # Only timeout failures
  ]
}' chainsaw-report.json > timeout-failures.json

./dependencies/upload-test-results.sh \
  --failures timeout-failures.json \
  --run-id "timeout-analysis"
```

### Validate Report Before Upload

```bash
#!/bin/bash
# Validate JSON structure before uploading

REPORT="test/chainsaw/chainsaw-report.json"

if ! jq empty "$REPORT" 2>/dev/null; then
    echo "ERROR: Invalid JSON in $REPORT"
    exit 1
fi

if ! jq -e '.tests | length > 0' "$REPORT" > /dev/null; then
    echo "WARNING: No tests in report"
fi

# Upload if valid
./dependencies/upload-test-results.sh --report "$REPORT"
```

### Dry Run (Check Without Uploading)

```bash
# Set read-only DB user (if available)
./dependencies/upload-test-results.sh \
  --db-user readonly \
  --skip-loki \
  2>&1 | tee upload-dry-run.log

# Or just validate the report
jq -r '.tests[] | "\(.name): \(.status // "unknown")"' \
  test/chainsaw/chainsaw-report.json
```

## Integration Examples

### Makefile Target

```makefile
.PHONY: upload-test-results
upload-test-results:
	@echo "Uploading test results..."
	./dependencies/upload-test-results.sh \
		--run-id "make-$(shell date +%s)" \
		--namespace "$(VCLUSTER_NAMESPACE)" \
		--installer manual
```

### Jenkins Pipeline

```groovy
stage('Upload Test Results') {
    steps {
        script {
            sh '''
                ./dependencies/upload-test-results.sh \
                    --report test/chainsaw/chainsaw-report.json \
                    --run-id "${BUILD_TAG}" \
                    --namespace "${VCLUSTER_NAMESPACE}" \
                    --installer helm
            '''
        }
    }
}
```

### Docker Container

```dockerfile
FROM postgres:15

RUN apt-get update && apt-get install -y \
    jq \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY dependencies/upload-test-results.sh /usr/local/bin/
COPY test/chainsaw/*.json /reports/

ENTRYPOINT ["/usr/local/bin/upload-test-results.sh"]
CMD ["--report", "/reports/chainsaw-report.json"]
```

## Best Practices

1. **Always specify `--run-id`** for production uploads
   - Makes results traceable
   - Prevents accidental overwrites with auto-generated IDs

2. **Include namespace information**
   - Essential for correlating with cluster logs
   - Use `--namespace` or `VCLUSTER_NAMESPACE`

3. **Use `--github-run` in CI**
   - Links test results back to CI run
   - Enables cross-referencing CI logs with test data

4. **Validate JSON before upload**
   - Use `jq` to check report structure
   - Prevents partial uploads with malformed data

5. **Handle upload failures gracefully**
   - Don't fail the entire pipeline if upload fails
   - Retry upload separately if needed
   - Keep report files as artifacts for manual upload

6. **Use port-forward for local development**
   - Safer than exposing database externally
   - Same credentials work both ways

## See Also

- [Test Observability Documentation](../test/chainsaw/README-observability.md) - Full integration guide
- [Test Runner Guide](../test/chainsaw/run-tests.sh) - Automated test execution
- [Monitoring Setup](monitoring/README.md) - Observability stack overview
