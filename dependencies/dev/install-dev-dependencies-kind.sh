echo "== Installing the dependencies == "

kustomize build --enable-helm overlays/kind-test | kubectl apply -f -
