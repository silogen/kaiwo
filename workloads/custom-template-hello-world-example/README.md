# Hello-world example of using custom templates

Kaiwo allows for passing custom templates and values for Jobs, Deployments, RayJobs and RayServices. This example shows how to add custom fields and values if the default templates embedded in the Kaiwo binary do not satisfy your needs. 

The example also illustrates how to 

1. use a kaiwoconfig which can be used instead of CLI arguments
2. pass a separate path for `envFile` which otherwise defaults to `env` in workload directory

Run this example with the following command:

`kaiwo submit -p workloads/custom-template-hello-world-example`