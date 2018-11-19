# Pod Resources Enforcer
A Kubernetes Dynamic Admission Controller that forces pods to specify resources request and limit, even when using LimitRange.

## Prerequisites

1. Docker (if you want to build it yourself)

2. Kubernetes 1.9.0 or above with the `admissionregistration.k8s.io/v1beta1` API enabled. Verify that by the following command:
```
kubectl api-versions | grep admissionregistration.k8s.io/v1beta1
```

In addition, the `ValidatingAdmissionWebhook` admission controller should be added and listed in the `enable-admission-control` flag of kube-apiserver.

## Build

If you want to build the application, you can use Docker by running:
```
cd ~/pod-resources-enforcer
docker build -t <IMAGE TAG> -f build/Dockerfile  .
```

Replace `<IMAGE TAG>` with your preffered container registry and image name.

## Deploy

1. Create a signed cert/key pair and store it in a Kubernetes `secret` that will be consumed by the admission controller.

You can use the `/scripts/create_certs.sh` script for ease-of-use:
```
./scripts/create_certs.sh \
    --service <SERVICE NAME> \
    --secret <SECRET NAME> \
    --namespace <NAMESPACE NAME>
```

for example:
```
./scripts/create_certs.sh \
    --service pod-resources-enforcer \
    --secret pod-resources-enforcer-cert \
    --namespace restricted
```

2. Patch the `ValidatingWebhookConfiguration` by setting `caBundle` with correct value from Kubernetes cluster.

The CA certificate should be provided so the `apiserver` can trust the TLS certificate of the webhook server. 

In case you sign certificates with the Kubernetes API, you can use the CA cert from your kubeconfig:
```
[~]# kubectl config view --raw -o json | jq -r '.clusters[0].cluster."certificate-authority-data"'
```

Or get it directly from the cluster by calling:

```
[~]# kubectl get configmap -n kube-system extension-apiserver-authentication -o=jsonpath='{.data.client-ca-file}' | base64 | tr -d '\n'
```

Edit the file `~/deployments/05-webhook-config.yaml` with the CA bundle value.

3. Deploy resources
```
# 1. Create the namespace to which the enforcer applies
[~]# kubectl create -f ~/deployments/01-namespace.yaml

# 2. Create a LimitRange object to enforce min and max values [Optional Step]
[~]# kubectl create -f ~/deployments/02-limitrange.yaml

# 3. Create the deployment
[~]# kubectl create -f ~/deployments/03-deployment.yaml

# 4. Create the service for the deployment
[~]# kubectl create -f ~/deployments/04-service.yaml

# 5. Create the validating webhook configuration
[~]# kubectl create -f ~/deployments/05-webhook-config.yaml
```

## Verify

1. The pod-resources-enforcer should be running in the created namespace
```
[~]# kubectl get pods -n restricted
NAME                                                  READY     STATUS    RESTARTS   AGE
pod-resources-enforcer-668f6f77c-l2l85                1/1       Running   0          1m

[~]# kubectl get deployment -n restricted
NAME                                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
pod-resources-enforcer                1         1         1            1           1m
```

2. Deploy test apps in Kubernetes cluster
```
cd ~/test

# 1. Create an application that doesn't specify resources request/limit
[~]# kubectl create -f 01-deploy-alpine-no-limits.yaml

# 2. Create an application that specifies resources request/limit outside of the LimitRange
[~]# kubectl create -f 02-deploy-alpine-bad-limits.yaml

# 3. Create an application that specifies valid resources request/limit 
[~]# kubectl create -f 03-deploy-alpine-good-limits.yaml
```

4. Verify policies were enforced
```
[~]# kubectl get deploy -n restricted
NAME                     DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
alpine-no-limits         1         0         0            0           1m
alpine-bad-limits        1         0         0            0           1m
alpine-good-limits       1         1         1            1           1m
pod-resources-enforcer   1         1         1            1           6m

```

Only the `alpine-good-limits` deployment should be running while the first two pods should be rejected (note that it has 0 deployed pods)

The `ReplicaSet` of each of the first two deployment will fail to deploy the pods, and you can see why:
```
[~]# kubectl describe rs alpine-bad-limits-5bbcc75f47 -n restricted
```

The look at the events at the bottom:
```
Type     Reason        Age               From                   Message
----     ------        ----              ----                   -------
Warning  FailedCreate  1m                replicaset-controller  Error creating: pods "alpine-bad-limits-5bbcc75f47-6m7fz" is forbidden: [maximum cpu usage per Container is 1, but limit is 4., maximum memory usage per Container is 2Gi, but limit is 4Gi.]
```

And a different message for the `alpine-no-limits-8956fcbc5`:

If `LimitRange` is applied:
```
Type     Reason        Age               From                   Message
----     ------        ----              ----                   -------
Warning  FailedCreate  1m                replicaset-controller  Error creating: pods "alpine-no-limits-8956fcbc5" is forbidden: [LimitRanger injected default resources request and limit which is forbidden]
```

Else if `LimitRange` is *not* applied:
```
Type     Reason        Age               From                   Message
----     ------        ----              ----                   -------
Warning  FailedCreate  1m                replicaset-controller  Error creating: pods "alpine-no-limits-8956fcbc5" is forbidden: [Container 'alpine-no-limits' does not have CPU and/or Memory requests defined]
```


## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.microsoft.com.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
