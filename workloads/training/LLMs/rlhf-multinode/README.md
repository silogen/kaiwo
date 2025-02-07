# RLHF (PPO) Multi-Node Training with OpenRLHF, Ray and vLLLM

Notice that number of replicas and GPUs per replica are hard coded in the entrypoint, currently totaling 12 gpus. If you change those values, remember to change the Kaiwo command below accordingly.   

`kaiwo submit -p workloads/training/LLMs/rlhf-multinode -g 12 --ray --storage=100Gi,nameofyourstorageclass`