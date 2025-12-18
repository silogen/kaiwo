# Longhorn Setup for Kubernetes Cluster

Longhorn is a distributed block storage system for Kubernetes that provides persistent storage with replication, snapshots, and backups.

## Prerequisites

### Node Requirements

Each node that will run Longhorn must have:

1. **open-iscsi installed** (for mounting volumes)
   ```bash
   # Ubuntu/Debian
   sudo apt-get install open-iscsi

   # RHEL/CentOS
   sudo yum install iscsi-initiator-utils

   # Verify
   sudo systemctl status iscsid
   ```

2. **NFSv4 client** (optional, for backup targets)
   ```bash
   # Ubuntu/Debian
   sudo apt-get install nfs-common

   # RHEL/CentOS
   sudo yum install nfs-utils
   ```

3. **Sufficient disk space** on nodes for storage
   - Longhorn will use available disk space on nodes
   - Default path: `/var/lib/longhorn/`
   - Ensure nodes have the required capacity for your storage needs

4. **Kernel modules**
   ```bash
   # Verify required modules are available
   sudo modprobe iscsi_tcp
   sudo modprobe dm_crypt
   ```

### Kubernetes Requirements

- Kubernetes v1.21 or later
- kubectl configured with cluster admin access
- Helm 3 (recommended installation method)

## Installation Methods

### Method 1: Helm (Recommended)

1. Add the Longhorn Helm repository:
   ```bash
   helm repo add longhorn https://charts.longhorn.io
   helm repo update
   ```

2. Review and customize the configuration:
   ```bash
   # See available configuration options
   helm show values longhorn/longhorn > longhorn-values.yaml
   ```

3. Edit `longhorn-values.yaml` or use `values.yaml` in this directory, then install:
   ```bash
   helm install longhorn longhorn/longhorn \
     --namespace longhorn-system \
     --create-namespace \
     --values values.yaml
   ```

### Method 2: kubectl apply

```bash
kubectl apply -f https://raw.githubusercontent.com/longhorn/longhorn/v1.7.0/deploy/longhorn.yaml
```

## Configuration

### Cluster-Specific Configuration

Before installing, you need to configure:

1. **Node Selection** - Which nodes should run Longhorn storage
2. **Disk Configuration** - Which disks/paths to use for storage
3. **Replication Settings** - Default number of replicas
4. **Resource Limits** - CPU and memory for Longhorn components

See `values.yaml` for a starter configuration template.

### Node and Disk Configuration

After installation, configure nodes and disks via the Longhorn UI or kubectl:

1. **Label nodes for Longhorn storage:**
   ```bash
   kubectl label nodes <node-name> node.longhorn.io/create-default-disk=true
   ```

2. **Configure custom disk paths per node:**
   ```bash
   # This is done via the Longhorn UI or by editing Node CRs
   kubectl edit node.longhorn.io <node-name> -n longhorn-system
   ```

3. **Disable default disk and add custom disks:**
   ```yaml
   apiVersion: longhorn.io/v1beta2
   kind: Node
   metadata:
     name: <node-name>
   spec:
     disks:
       disk-1:
         path: /mnt/storage/longhorn
         allowScheduling: true
         evictionRequested: false
         storageReserved: 10737418240  # 10GB in bytes
         tags:
         - fast
         - ssd
   ```

## Post-Installation

### Access Longhorn UI

1. **Port-forward method** (for testing):
   ```bash
   kubectl port-forward -n longhorn-system svc/longhorn-frontend 8080:80
   # Access at http://localhost:8080
   ```

2. **Ingress method** (for production):
   ```bash
   kubectl apply -f ingress.yaml
   ```

### Create StorageClass

Longhorn automatically creates a default StorageClass named `longhorn`. To create custom StorageClasses:

### Verify Installation

```bash
# Check all Longhorn pods are running
kubectl get pods -n longhorn-system

# Check Longhorn nodes
kubectl get nodes.longhorn.io -n longhorn-system

# Check available StorageClasses
kubectl get storageclass
```

### Test Volume Creation

```bash
kubectl apply -f test-pvc.yaml
kubectl get pvc
```

## Backup Configuration

Configure S3-compatible backup target:

```bash
# Via kubectl
kubectl edit settings.longhorn.io backup-target -n longhorn-system
```

Example backup targets:
- S3: `s3://bucket-name@region/path/`
- NFS: `nfs://nfs-server:/path/`

See `backup-secret.yaml` for S3 credentials configuration.

## Monitoring

Longhorn provides Prometheus metrics. To enable monitoring:

1. Install Prometheus/Grafana if not already present
2. Configure ServiceMonitor:
   ```bash
   kubectl apply -f servicemonitor.yaml
   ```

## Maintenance

### Upgrade Longhorn

```bash
helm upgrade longhorn longhorn/longhorn \
  --namespace longhorn-system \
  --values values.yaml
```

### Backup Volumes

Longhorn supports:
- Manual snapshots (via UI or CRDs)
- Recurring snapshot/backup policies
- Cross-cluster disaster recovery

## Uninstallation

**WARNING: This will delete all Longhorn volumes!**

```bash
# Delete all volumes first
kubectl delete pvc --all

# Uninstall Longhorn
helm uninstall longhorn -n longhorn-system

# Clean up CRDs
kubectl delete crd $(kubectl get crd | grep longhorn | awk '{print $1}')

# Clean up namespace
kubectl delete namespace longhorn-system
```

## Troubleshooting

### Common Issues

1. **Pods stuck in Pending:**
   - Check if open-iscsi is installed and running on nodes
   - Verify disk space is available

2. **Volume attachment failures:**
   - Check node connectivity
   - Verify iscsi_tcp kernel module is loaded

3. **Performance issues:**
   - Check disk I/O performance
   - Review replica count (lower = faster, but less redundant)
   - Consider using local SSDs

### Logs

```bash
# Manager logs
kubectl logs -n longhorn-system -l app=longhorn-manager

# UI logs
kubectl logs -n longhorn-system -l app=longhorn-ui

# Driver logs
kubectl logs -n longhorn-system -l app=longhorn-driver-deployer
```

## References

- [Longhorn Documentation](https://longhorn.io/docs/)
- [Longhorn GitHub](https://github.com/longhorn/longhorn)
- [Best Practices](https://longhorn.io/docs/latest/best-practices/)
