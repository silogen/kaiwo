echo "== Ensuring the AMD GPU operator is installed =="

helm upgrade --install amd-gpu-operator rocm/gpu-operator-charts \
  --namespace kube-amd-gpu \
  --create-namespace \
  --version=v1.3.0

echo "== Installing the dependencies == "

kubectl kustomize --enable-helm overlays/amd-gpu | kubectl apply -f -
