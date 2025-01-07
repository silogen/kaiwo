#TODO

- describe pre-requisites 
  - k8s: at minimum cert-manager, AMD device plugin, AMD node labeller (recommended: prometheus, grafana, loki)
  - local: kubectl, helm (only for install)
- describe how to install minimum prerequisites (Cluster-Forge or applying manifests with `.\install-mini-prerequisites.sh`)
- describe how to install the AI Workload Orchestrator (Cluster-Forge or applying manifests with `install-ray-kueue-operators.sh`)
- remember to mention that manifests for Kueue and Ray operators are in cluster-forge input/ directory IF the user chooses to use cluster-forge
- Ray operator
- Kueue operator
- Kueue resource flavour(s)
- Kueue Cluster Queue
- Kueue Local Queue