# Kaiwo GitOps Repository

This repository contains the GitOps configuration for deploying Kaiwo and its dependencies using Flux v2.

## Repository Structure

```
kaiwo-gitops/
├── README.md                          # This file
├── SETUP.md                          # Detailed setup instructions  
├── ansible/                          # Ansible playbooks for node setup
│   ├── amd-rocm.yaml                # AMD GPU drivers and ROCm
│   ├── rke.yaml                     # RKE2 Kubernetes cluster
│   └── inventory.ini.example        # Inventory template
├── flux/                            # Flux-specific resources
│   ├── clusters/                    # Cluster-specific configurations
│   │   ├── dev-amd-gpu/             # Development AMD GPU cluster
│   │   │   ├── flux-system/         # Flux bootstrap files
│   │   │   └── infrastructure.yaml  # Infrastructure apps entrypoint
│   │   └── dev-kind/                # Development KIND cluster
│   ├── helm/                        # Helm releases
│   │   ├── amd-gpu-operator/        # AMD GPU operator Helm release
│   │   └── nfs-storage/             # NFS storage provisioner
│   ├── profiles/                    # Flux Kustomizations (orchestration)
│   │   ├── base/                    # Base infrastructure components
│   │   ├── dev-amd-gpu/             # AMD GPU cluster profile
│   │   └── dev-kind/                # KIND development profile
│   └── sources/                     # Git repositories and Helm repos
└── kustomization/                   # Pure Kustomize resources
    ├── cert-manager/                # Certificate management
    ├── kueue/                       # Job queueing system
    ├── kuberay/                     # Ray distributed computing
    ├── appwrapper/                  # Workload batching
    ├── amd-gpu-device-config/       # AMD GPU device configuration
    ├── nvidia-fake-gpu/             # Fake GPU for testing
    ├── monitoring/                  # Observability components
    └── otel-*/                      # OpenTelemetry components
```

## Architecture

**Separation of Concerns:**
- **`flux/`** - Flux-specific resources (Kustomizations, HelmReleases, Sources, Clusters)
- **`kustomization/`** - Pure Kustomize manifests (static YAML, patches, configs)
- **`ansible/`** - Node-level setup (OS, drivers, Kubernetes)

**Deployment Flow:**
1. **Ansible** sets up nodes (AMD drivers, RKE2 cluster)
2. **Flux** is bootstrapped pointing to `flux/clusters/dev-amd-gpu/` or `flux/clusters/dev-kind/`
3. **Flux Kustomizations** in `flux/profiles/` orchestrate deployments
4. **Static manifests** in `kustomization/` provide the actual resources
5. **Helm releases** in `flux/helm/` deploy complex applications

## Quick Start

### 1. Setup Infrastructure
```bash
cd ansible
cp inventory.ini.example inventory.ini
# Edit inventory.ini with your server IPs
ansible-playbook -i inventory.ini amd-rocm.yaml
ansible-playbook -i inventory.ini rke.yaml
```

### 2. Bootstrap Flux

For **private repositories** (recommended):
```bash
export GITHUB_TOKEN=<your-personal-access-token>
export GITHUB_USER=<your-username>
export GITHUB_REPO=kaiwo-gitops

flux bootstrap github \
  --owner=$GITHUB_USER \
  --repository=$GITHUB_REPO \
  --branch=main \
  --path=./flux/clusters/dev-amd-gpu \
  --private
```

For **public repositories** (read-only deployment):
```bash
export GITHUB_USER=<your-username>
export GITHUB_REPO=kaiwo-gitops

flux bootstrap github \
  --owner=$GITHUB_USER \
  --repository=$GITHUB_REPO \
  --branch=main \
  --path=./flux/clusters/dev-amd-gpu \
  --read-write-key=false
```

### 3. Monitor Deployment
```bash
# Wait for Flux to create the infrastructure kustomization first
kubectl wait --for=condition=ready kustomization/flux-system -n flux-system --timeout=300s

# Wait for infrastructure to be ready
kubectl wait --for=condition=ready kustomization/kaiwo-infrastructure -n flux-system --timeout=600s

# Check status
flux get all
kubectl get pods -A
```

## Profiles

- **dev-kind**: Minimal setup for local KIND cluster with fake GPUs
- **dev-amd-gpu**: Development setup with AMD GPU support and monitoring
- **base**: Core infrastructure components (cert-manager, kueue, kuberay, etc.)

## Customization

1. **Node setup**: Modify `ansible/` playbooks for your hardware
2. **Infrastructure**: Add components to `kustomization/` and reference in `flux/profiles/`
3. **Helm apps**: Add releases to `flux/helm/` and include in profiles
4. **Environment-specific**: Create new profiles in `flux/profiles/`

## Monitoring

```bash
# Check Flux status
flux get all

# Force reconciliation  
flux reconcile kustomization kaiwo-infrastructure --with-source

# Check logs
flux logs --kind=Kustomization --name=kaiwo-infrastructure

# Check resources
kubectl get helmreleases,kustomizations -A
```