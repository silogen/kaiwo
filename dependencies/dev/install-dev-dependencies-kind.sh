echo "== Installing the dependencies == "

kubectl kustomize --enable-helm overlays/kind-test | kubectl apply -f -
