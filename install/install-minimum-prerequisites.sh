#!/bin/bash

# Assuming no tweaking is needed for the cert-manager installation
# kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml

# AMD Operator
helm repo add amd-gpu-operator https://rocm.github.io/gpu-operator 
helm repo update 
helm install amd-gpu-operator amd-gpu-operator/gpu-operator-charts --namespace kube-amd-gpu --create-namespace 
kubectl apply -f ./deviceconfig.yaml

# Prometheus
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts && helm repo update 
helm --namespace prometheus-system install prometheus prometheus-community/kube-prometheus-stack --create-namespace --version 48.2.1 -f ./prometheus/prometheus-overrides.yaml 
kubectl apply -f ./prometheus/podMonitor.yaml 
kubectl apply -f ./prometheus/prometheusRules.yaml

