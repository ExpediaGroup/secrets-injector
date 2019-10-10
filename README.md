# Secrets Injector

Uses a mutating webhook to create an `initContainer` within an annotated pod 

The mutatingwebhook configuration limits to an annotated namespace 

The `initContainer` can be of any secret image as desired, but the examples folder gives a simple python
script for AWS

The image names are overridden in the helm chart supplied and the command and args to fire are also configurable

The mount point of the secrets obtained is configurable and is mounted as an in-memory volume from the `initContainer`
to the `Volumes` in the pod to be injected.


Acknowledgments to: https://banzaicloud.com/blog/k8s-admission-webhooks/

# Kubernetes Admission Webhook example

This tutorial shows how to build and deploy an [AdmissionWebhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#admission-webhooks).

## Building

### Build the go binary `secrets-injector` 
`make go-build`

### Package the secrets injector binary into a docker image 
`make docker-injector-build`
`make docker-injector-build INJECTOR_TAG=myrepotag`

### Package the example aws secret reader into a docker image 
`make docker-secret-build`
`make docker-secret-build SECRET_TAG=myrepotag`

### Push to docker repo
`make docker-push INJECTOR_TAG=myrepotag SECRET_TAG=myrepotag`

### Helm install
`make helm-install INJECTOR_TAG=myrepotag SECRET_TAG=myrepotag`
Put any additional overrides in overrides.yaml in helm folder

### All
`make all INJECTOR_TAG=myrepotag SECRET_TAG=myrepotag`

## Setup

### Decorate namespace with a label

`kubectl label namespace default com.expediagroup/secrets-injector=enabled`

This is overridable in the helm chart if necessary `namespaceSelector: mylabel`

### Add pod labels

`com.expediagroup/secrets-injector-format: yaml`
`com.expediagroup/secrets-injector-key: my-secret-key`

### Create IAM role/policies/trust

### IAM Role Example Policy
```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": [
                "secretsmanager:*"
            ],
            "Resource": "*",
            "Effect": "Allow"
        }
    ]
}
```

### IAM Role Example Trust
```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {
        "Service": "secretsmanager.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::000000000000:role/eks-worker-role"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```


## Installation

### Use helm chart to create the manifests either via helm install or template

`helm install -n myrelease --set image.pullPolicy=Never --set image.repository=myrepo helm/secrets-injector`

- helm install will automatically build certs/ca and create mountable secrets
- helm overrides will allow launch of a specified image for both the webhook and the secret reading process

### Override aws secret and/or region with helm values

```
aws:
  secret:
    key: my-key
  region: us-west-2
```

## Debugging

### Examine events
`kubectl get events` to check

### Create a sample set of pods (sleep.yaml is in examples/ folder)

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sleep
spec:
  replicas: 3
  selector:
    matchLabels:
      app: sleep
  template:
    metadata:
      annotations:
        iam.amazonaws.com/role: arn:aws:iam::000000000000:role/my-secret-role
      labels:
        com.expediagroup/secrets-injector-format: yaml
        com.expediagroup/secrets-injector-key: s-eg-platform
        app: sleep
    spec:
      containers:
      - name: sleep
        image: tutum/curl
        command: ["/bin/sleep","infinity"]
        imagePullPolicy: IfNotPresent
```

### Then check the resultant pod by

`kubectl exec sleep-xxx cat /secrets/secret.yaml`
