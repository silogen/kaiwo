#  Simple example to train a Bert model on a classification task


## Overview

The `gpus` field is passed to the job as `NUM_GPUS` environment variable in the entrypoint so you don't need to touch the entrypoint when adding GPUs for training.

Run example with:

`kubectl apply -f kaiwojob-bert-accelerate.yaml`

Or if you're using kaiwo-cli which can also set user email and clusterQueue to the correct one, run

`kaiwo submit -f kaiwojob-bert-accelerate.yaml`

## Dependencies
- None if using Bert