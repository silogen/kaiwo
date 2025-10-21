# API Reference

## Packages
- [infrastructure.silogen.ai/v1alpha1](#infrastructuresilogenaiv1alpha1)


## infrastructure.silogen.ai/v1alpha1

Package v1alpha1 contains API Schema definitions for the infrastructure v1alpha1 API group.

### Resource Types
- [NodePartitioning](#nodepartitioning)
- [NodePartitioningList](#nodepartitioninglist)
- [PartitioningPlan](#partitioningplan)
- [PartitioningPlanList](#partitioningplanlist)
- [PartitioningProfile](#partitioningprofile)
- [PartitioningProfileList](#partitioningprofilelist)







#### NodePartitioning



NodePartitioning is the Schema for the nodepartitionings API.
It represents a per-node work item for applying a partition profile.



_Appears in:_
- [NodePartitioningList](#nodepartitioninglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `NodePartitioning` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NodePartitioningSpec](#nodepartitioningspec)_ |  |  |  |
| `status` _[NodePartitioningStatus](#nodepartitioningstatus)_ |  |  |  |




#### NodePartitioningList



NodePartitioningList contains a list of NodePartitioning.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `NodePartitioningList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[NodePartitioning](#nodepartitioning) array_ |  |  |  |


#### NodePartitioningPhase

_Underlying type:_ _string_

NodePartitioningPhase represents the current phase of node partitioning.

_Validation:_
- Enum: [Pending Draining Applying WaitingOperator Verifying Succeeded Failed Skipped]

_Appears in:_
- [NodePartitioningHistoryEntry](#nodepartitioninghistoryentry)
- [NodePartitioningStatus](#nodepartitioningstatus)
- [NodeStatusSummary](#nodestatussummary)

| Field | Description |
| --- | --- |
| `Pending` | NodePartitioningPhasePending indicates the operation has not started yet.<br /> |
| `Draining` | NodePartitioningPhaseDraining indicates the node is being drained.<br /> |
| `Applying` | NodePartitioningPhaseApplying indicates the DCM profile is being applied.<br /> |
| `WaitingOperator` | NodePartitioningPhaseWaitingOperator indicates waiting for the GPU operator to reconcile.<br /> |
| `Verifying` | NodePartitioningPhaseVerifying indicates verification is in progress.<br /> |
| `Succeeded` | NodePartitioningPhaseSucceeded indicates the operation completed successfully.<br /> |
| `Failed` | NodePartitioningPhaseFailed indicates the operation failed.<br /> |
| `Skipped` | NodePartitioningPhaseSkipped indicates the operation was skipped (dry-run or paused).<br /> |


#### NodePartitioningSpec



NodePartitioningSpec defines the desired state of NodePartitioning.



_Appears in:_
- [NodePartitioning](#nodepartitioning)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `planRef` _[PlanReference](#planreference)_ | PlanRef references the parent PartitioningPlan. |  |  |
| `dryRun` _boolean_ | DryRun indicates whether this is a dry-run operation.<br />When true, the controller will skip all actual operations and set phase to Skipped. |  |  |
| `nodeName` _string_ | NodeName is the name of the target node. |  | MinLength: 1 <br /> |
| `desiredHash` _string_ | DesiredHash is a deterministic hash of the desired partition state.<br />Used for idempotency and change detection. |  | MinLength: 1 <br /> |
| `profileRef` _[ProfileReference](#profilereference)_ | ProfileRef references the PartitioningProfile to apply. |  |  |


#### NodePartitioningStatus



NodePartitioningStatus defines the observed state of NodePartitioning.



_Appears in:_
- [NodePartitioning](#nodepartitioning)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `phase` _[NodePartitioningPhase](#nodepartitioningphase)_ | Phase represents the current phase in the state machine. |  | Enum: [Pending Draining Applying WaitingOperator Verifying Succeeded Failed Skipped] <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the operation's state. |  |  |
| `currentHash` _string_ | CurrentHash is the hash of the current applied state. |  |  |


#### NodeStatusSummary



NodeStatusSummary provides a lightweight summary of a node's partitioning status.



_Appears in:_
- [PartitioningPlanStatus](#partitioningplanstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeName` _string_ | NodeName is the name of the node. |  |  |
| `desiredHash` _string_ | DesiredHash is the desired state hash. |  |  |
| `currentHash` _string_ | CurrentHash is the current state hash. |  |  |
| `phase` _[NodePartitioningPhase](#nodepartitioningphase)_ | Phase is the current phase. |  | Enum: [Pending Draining Applying WaitingOperator Verifying Succeeded Failed Skipped] <br /> |
| `retries` _integer_ | Retries is the number of retry attempts so far. |  |  |
| `lastError` _string_ | LastError is the most recent error message. |  |  |
| `lastUpdateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | LastUpdateTime is the timestamp of the last status update. |  |  |


#### PartitioningPlan



PartitioningPlan is the Schema for the partitioningplans API.
It orchestrates the application of partition profiles across a set of nodes.



_Appears in:_
- [PartitioningPlanList](#partitioningplanlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `PartitioningPlan` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PartitioningPlanSpec](#partitioningplanspec)_ |  |  |  |
| `status` _[PartitioningPlanStatus](#partitioningplanstatus)_ |  |  |  |


#### PartitioningPlanList



PartitioningPlanList contains a list of PartitioningPlan.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `PartitioningPlanList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[PartitioningPlan](#partitioningplan) array_ |  |  |  |


#### PartitioningPlanPhase

_Underlying type:_ _string_

PartitioningPlanPhase represents the overall phase of a partitioning plan.

_Validation:_
- Enum: [Pending Progressing Paused Completed Degraded]

_Appears in:_
- [PartitioningPlanStatus](#partitioningplanstatus)

| Field | Description |
| --- | --- |
| `Pending` | PartitioningPlanPhasePending indicates the plan has not started yet.<br /> |
| `Progressing` | PartitioningPlanPhaseProgressing indicates the plan is actively reconciling.<br /> |
| `Paused` | PartitioningPlanPhasePaused indicates the plan is paused.<br /> |
| `Completed` | PartitioningPlanPhaseCompleted indicates all nodes have succeeded.<br /> |
| `Degraded` | PartitioningPlanPhaseDegraded indicates some nodes have failed.<br /> |


#### PartitioningPlanSpec



PartitioningPlanSpec defines the desired state of PartitioningPlan.



_Appears in:_
- [PartitioningPlan](#partitioningplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `paused` _boolean_ | Paused indicates whether the plan should stop reconciling.<br />When true, no new node operations will be started, but status updates continue. |  |  |
| `dryRun` _boolean_ | DryRun indicates whether to simulate the plan without making any changes.<br />When true, NodePartitioning resources are created but marked as Skipped. |  |  |
| `rollout` _[RolloutPolicy](#rolloutpolicy)_ | Rollout defines how to orchestrate node changes. |  |  |
| `rules` _[PartitioningRule](#partitioningrule) array_ | Rules defines the rules mapping nodes to partition profiles. |  | MinItems: 1 <br /> |


#### PartitioningPlanStatus



PartitioningPlanStatus defines the observed state of PartitioningPlan.



_Appears in:_
- [PartitioningPlan](#partitioningplan)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `phase` _[PartitioningPlanPhase](#partitioningplanphase)_ | Phase represents the overall phase of the plan. |  | Enum: [Pending Progressing Paused Completed Degraded] <br /> |
| `summary` _[PlanSummary](#plansummary)_ | Summary aggregates node counts by phase. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the plan's state. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#time-v1-meta)_ | LastSyncTime is the timestamp of the last successful reconciliation. |  |  |
| `nodeStatuses` _[NodeStatusSummary](#nodestatussummary) array_ | NodeStatuses is a lightweight cache of per-node status for dashboards.<br />The source of truth is in the NodePartitioning resources. |  |  |


#### PartitioningProfile



PartitioningProfile is the Schema for the partitioningprofiles API.
It defines a reusable GPU partition configuration that can be referenced by PartitioningPlans.



_Appears in:_
- [PartitioningProfileList](#partitioningprofilelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `PartitioningProfile` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PartitioningProfileSpec](#partitioningprofilespec)_ |  |  |  |
| `status` _[PartitioningProfileStatus](#partitioningprofilestatus)_ |  |  |  |


#### PartitioningProfileList



PartitioningProfileList contains a list of PartitioningProfile.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infrastructure.silogen.ai/v1alpha1` | | |
| `kind` _string_ | `PartitioningProfileList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[PartitioningProfile](#partitioningprofile) array_ |  |  |  |


#### PartitioningProfileSpec



PartitioningProfileSpec defines the desired state of PartitioningProfile.



_Appears in:_
- [PartitioningProfile](#partitioningprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | Description is a human-readable description of this profile. |  |  |
| `targetSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#labelselector-v1-meta)_ | TargetSelector is an optional guardrail to ensure the profile is only<br />applied to compatible nodes. If specified, the controller will validate<br />that nodes match this selector before applying the profile. |  |  |
| `expectedResources` _object (keys:string, values:[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#quantity-resource-api))_ | ExpectedResources defines the resources that should appear in<br />node.status.allocatable after partitioning succeeds. |  |  |
| `dcmProfileName` _string_ | DcmProfileName is the name of the profile to be applied.<br />This is the name of the profile under `gpu-config-profile` in DCM config.json. |  |  |


#### PartitioningProfileStatus



PartitioningProfileStatus defines the observed state of PartitioningProfile.



_Appears in:_
- [PartitioningProfile](#partitioningprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed by the controller. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#condition-v1-meta) array_ | Conditions represent the latest available observations of the profile's state. |  |  |


#### PartitioningRule



PartitioningRule maps a node selector to a partition profile.



_Appears in:_
- [PartitioningPlanSpec](#partitioningplanspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | Description is an optional human-readable name for this rule. |  |  |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#labelselector-v1-meta)_ | Selector selects nodes to partition. |  |  |
| `profileRef` _[ProfileReference](#profilereference)_ | ProfileRef references the PartitioningProfile to apply. |  |  |


#### PlanReference



PlanReference references the parent PartitioningPlan.



_Appears in:_
- [NodePartitioningSpec](#nodepartitioningspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the PartitioningPlan. |  |  |
| `uid` _string_ | UID of the PartitioningPlan (for strong ownership). |  |  |


#### PlanSummary



PlanSummary aggregates node counts by phase.



_Appears in:_
- [PartitioningPlanStatus](#partitioningplanstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `totalNodes` _integer_ | TotalNodes is the total number of nodes tracked by the plan. |  |  |
| `matchingNodes` _integer_ | MatchingNodes is the number of nodes matched by the plan's selectors. |  |  |
| `pending` _integer_ | Pending is the number of nodes in Pending phase. |  |  |
| `applying` _integer_ | Applying is the number of nodes in Applying or related phases. |  |  |
| `verifying` _integer_ | Verifying is the number of nodes in Verifying phase. |  |  |
| `succeeded` _integer_ | Succeeded is the number of nodes that completed successfully. |  |  |
| `failed` _integer_ | Failed is the number of nodes that failed. |  |  |
| `skipped` _integer_ | Skipped is the number of nodes that were skipped. |  |  |


#### ProfileReference



ProfileReference references a PartitioningProfile.



_Appears in:_
- [NodePartitioningSpec](#nodepartitioningspec)
- [PartitioningRule](#partitioningrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the PartitioningProfile. |  | MinLength: 1 <br /> |


#### RolloutPolicy



RolloutPolicy defines how to orchestrate the rollout across multiple nodes.



_Appears in:_
- [PartitioningPlanSpec](#partitioningplanspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxParallel` _integer_ | MaxParallel is the maximum number of nodes to process in parallel. | 1 | Minimum: 1 <br /> |
| `maxUnavailable` _integer_ | MaxUnavailable is the maximum number of nodes allowed to be unavailable at once.<br />Unavailable means: Draining, Applying, Verifying, or Failed. | 1 | Minimum: 0 <br /> |
| `excludeControlPlane` _boolean_ | ExcludeControlPlane controls if control plane nodes are excluded or not. | true |  |


