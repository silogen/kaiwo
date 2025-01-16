#  Simple example to train a Bert model on a classification task

The `--gpus`/`-g` is passed to the job as `NUM_GPUS` environment variable in the entrypoint so you don't need to touch the entrypoint when adding GPUs for training.

To run on 4 GPUs:

`kaiwo submit -p workloads/training/LLMs/jobs/single-node-bert-train-classification -g 4`