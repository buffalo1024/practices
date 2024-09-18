a practice project about admission

you can have a try by several steps

1. create namespace test

2. apply the rbac files, include service, clusterrole, clusterrolebinding

3. apply the deployment.yaml file

4. label the namespace you will apply a test workload in, the label key is test-webhook, you can label a namespace like 'kubectl label namespace default test-webhook=on'

5. apply workload in the namespace labeled in step 4, only deployments supported now

notice:
  you can deploy this project into other namespace, remember modify the deployment.yaml and the rbac yaml files to match the case.