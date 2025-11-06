# Image Error Detection Tests

Tests for validating that image errors are properly detected, categorized, and surfaced across the entire stack (Model → Template → Service).

## Test Scenarios

### 1. Not Found (`not-found/`)

**Scenario**: Service created with non-existent image URL

**Expected Flow**:
1. AIMService creates auto-generated AIMModel from `spec.model.image`
2. AIMModel attempts to inspect image and fails with 404/not found
3. AIMModel status → `Degraded` with condition `MetadataExtracted=False`, reason `ImageNotFound`
4. Auto-generated AIMServiceTemplate attempts discovery and fails (ImagePullBackOff)
5. Template status → `Degraded` with condition `Failure=True`, reason `ImageNotFound`
6. AIMService observes template is degraded
7. Service status → `Degraded` with condition `Failure=True`

**Key Assertions**:
- Model condition: `MetadataExtracted=False` with `reason: ImageNotFound`
- Model message contains: "not found", "404", or "manifest unknown"
- Template condition: `Failure=True` with `reason: ImageNotFound`
- Template message contains: "ImagePullBackOff" or "ErrImagePull"
- Service condition: `RuntimeReady=False`

### 2. Auth Required (`auth-required/`)

**Scenario**: Service created with private image URL (no credentials)

**Expected Flow**:
1. AIMService creates auto-generated AIMModel from `spec.model.image`
2. AIMModel attempts to inspect image and fails with 401/403/unauthorized
3. AIMModel status → `Degraded` with condition `MetadataExtracted=False`, reason `ImagePullAuthFailure`
4. Auto-generated AIMServiceTemplate attempts discovery and fails (ImagePullBackOff with auth error)
5. Template status → `Degraded` with condition `Failure=True`, reason `ImagePullAuthFailure`
6. AIMService observes template is degraded
7. Service status → `Degraded` with condition `Failure=True`

**Key Assertions**:
- Model condition: `MetadataExtracted=False` with `reason: ImagePullAuthFailure`
- Model message contains: "unauthorized", "401", "403", "denied", "authentication", or "credentials"
- Template condition: `Failure=True` with `reason: ImagePullAuthFailure`
- Template message contains: "ImagePullBackOff" or "ErrImagePull"
- Service condition: `RuntimeReady=False`

## Test Structure

Following the standard Chainsaw test structure:

```
service/errors/image-errors/
├── not-found/
│   ├── service.yaml          # AIMService with non-existent image
│   └── chainsaw-test.yaml    # Test assertions
├── auth-required/
│   ├── service.yaml          # AIMService with private image
│   └── chainsaw-test.yaml    # Test assertions
└── README.md
```

## Running the Tests

```bash
# Run all image error tests
chainsaw test --test-dir test/chainsaw/tests/aim/service/errors/image-errors

# Run specific scenario
chainsaw test --test-dir test/chainsaw/tests/aim/service/errors/image-errors/not-found
chainsaw test --test-dir test/chainsaw/tests/aim/service/errors/image-errors/auth-required
```

## Error Categorization

The operator categorizes image pull errors by analyzing error messages:

### ImagePullAuthFailure
Triggered by keywords: `unauthorized`, `authentication required`, `401`, `403`, `forbidden`, `denied`, `credentials`

**Sources**:
- Registry API returns 401 Unauthorized or 403 Forbidden
- Missing or invalid imagePullSecrets
- Expired registry credentials

### ImageNotFound
Triggered by keywords: `not found`, `404`, `manifest unknown`, `name unknown`

**Sources**:
- Image tag doesn't exist
- Image repository doesn't exist
- Registry typo in image URL
- Image was deleted

### ImagePullBackOff (Generic)
Triggered by any other image pull failure

**Sources**:
- Network connectivity issues
- Registry temporarily unavailable
- Malformed image reference
- Other transient failures

## Implementation Details

### Model-Level Detection
**File**: `internal/controller/aim/shared/image_inspector.go`

```go
// InspectImage wraps registry errors in ImageRegistryError
func InspectImage(...) (*ImageMetadata, error) {
    // ...
    if err := remote.Get(...); err != nil {
        return nil, &ImageRegistryError{
            Type: categorizeRegistryError(err),
            Message: fmt.Sprintf("failed to fetch image: %v", err),
        }
    }
}
```

**Status**: `internal/controller/aim/shared/image.go`
```go
// ProjectImageStatus sets model to Degraded with appropriate reason
if registryErr != nil {
    var reason string
    switch registryErr.Type {
    case ImagePullErrorAuth:
        reason = AIMModelReasonImagePullAuthFailure
    case ImagePullErrorNotFound:
        reason = AIMModelReasonImageNotFound
    }
    // Set conditions and Degraded status
}
```

### Template-Level Detection
**File**: `internal/controller/aim/shared/template.go`

```go
// checkJobPodImagePullStatus checks discovery job pods
func checkJobPodImagePullStatus(...) *ImagePullError {
    // Check pod.Status.ContainerStatuses for ImagePullBackOff/ErrImagePull
    if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
        return &ImagePullError{
            Type: categorizeImagePullError(message),
            // ...
        }
    }
}
```

**Status**: `internal/controller/aim/shared/template.go`
```go
// handleTemplateImagePullFailed sets template to Degraded
switch pullErr.Type {
case ImagePullErrorAuth:
    conditionReason = "ImagePullAuthFailure"
case ImagePullErrorNotFound:
    conditionReason = "ImageNotFound"
default:
    conditionReason = "ImagePullBackOff"
}
```

### Service-Level Propagation
**File**: `internal/controller/aim/shared/service_status.go`

```go
// HandleTemplateDegraded propagates template errors to service
if obs.TemplateStatus.Status == Degraded {
    status.Status = AIMServiceStatusDegraded
    // Copy failure condition from template
}
```

## Notes

- All image errors result in **`Degraded`** status (not `Failed`) as they are potentially recoverable
- Tests use realistic image URLs to trigger actual Kubernetes/registry errors
- Auto-generated resource names use regex patterns to match naming conventions
- Timeouts set to 180s to accommodate image pull attempts and retries
- Tests verify error propagation through the entire stack: Model → Template → Service
