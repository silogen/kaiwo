# Instructions for maintainers

## Running locally

1. To run the operator locally, you need to have a Kubernetes cluster running. You can use [kind](https://kind.sigs.k8s.io/) to create a local cluster.
2. To run locally with webhooks enabled, you need certificates. You can generate them using the following commands:

```shell
mkdir -p certs
# export the CAROOT env variable, used by the mkcert tool to generate CA and certs
export CAROOT=$(pwd)/certs
# the following command will create 2 files rootCA.pem and rootCA-key.pem
mkcert -install
# here, we're creating certificates valid for both "host.docker.internal" for MacOS and "172.17.0.1" for Linux
mkcert -cert-file=$CAROOT/tls.crt -key-file=$CAROOT/tls.key host.docker.internal 172.17.0.1
```

Copy the generated certs with the following commands:

```shell
# for MacOS
cat certs/rootCA.pem | base64

# for Linux
cat certs/rootCA.pem | base64 -w 0
```

Paste the output in the `caBundle` fields in the webhook configurations in `config/webhook_local_dev/webhooks.taml` file.

Export `WEBHOOK_CERT_DIRECTORY=path/to/certs` or add it to .env file for IDE to pick up the env variable.

3. Apply the manifests:

```shell
kubectl apply -f config/webhook_local_dev/webhooks.yaml
```

4. Start your IDE and run the operator.

You make have to also add KUBECONFIG env variable to your IDE to point to the kubeconfig file of your local cluster. With kind, if you created a cluster with `kind create cluster -n my-cluster`, you can ask kind to output the kubeconfig file with `kind get kubeconfig --name my-cluster > my-cluster-kubeconfig.yaml`.

