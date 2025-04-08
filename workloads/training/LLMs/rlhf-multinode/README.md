# RLHF (PPO) Multi-Node Training with OpenRLHF, Ray and vLLLM

## Pre-requisites

Make sure you have S3 endpoint and credentials set up in your environment. If you don't, you can deploy a minio instance with `workloads/dev-storage/s3-deployment.yaml`

Run with kubectl:   

`kubectl apply -f kaiwojob-llama-3-8b-mixture.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-llama-3-8b-mixture.yaml`