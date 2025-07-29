# Ansible Configuration

## Setup

1. Copy the example inventory and customize with your server IPs:
   ```bash
   cp inventory.ini.example inventory.ini
   # Edit inventory.ini with your actual server IPs and SSH settings
   ```

2. Run the playbooks:
   ```bash
   # Install AMD ROCm on both nodes
   ansible-playbook -i inventory.ini amd-rocm.yaml
   
   # Setup RKE2 cluster  
   ansible-playbook -i inventory.ini rke.yaml
   ```

## Files

- `inventory.ini.example` - Template inventory file
- `inventory.ini` - Your actual inventory with real IPs (gitignored)
- `amd-rocm.yaml` - Installs AMD GPU drivers and ROCm
- `rke.yaml` - Sets up RKE2 Kubernetes cluster