# kubergrunt

`kubergrunt` is an encompassing tool that attempts to fill in the gaps between Terraform, Helm, and Kubectl for managing
a Kubernetes Cluster. The binaries are all built as part of the CI pipeline on each release of the package, and is
appended to the corresponding release in the [Releases Page](/../../releases).

Some of the features of `kubergrunt` includes:

* configuring `kubectl` to authenticate with a given EKS cluster. Learn more about authenticating `kubectl` to EKS
  in the [eks-cluster module README](../eks-cluster/README.md#how-to-authenticate-kubectl).
* managing Helm and associated TLS certificates.
* setting up Helm client with TLS certificates.


## Installation

You can install `kubergrunt` using the [Gruntwork Installer](https://github.com/gruntwork-io/gruntwork-installer):

```bash
gruntwork-install --binary-name "kubergrunt" --repo "https://github.com/gruntwork-io/kubergrunt" --tag "v0.0.1"
```

Alternatively, you can download the corresponding binary for your platform directly from the [Releases Page](/../../releases).


## Commands

The following commands are available as part of `kubergrunt`:

1. [eks](#eks)
    * [configure](#configure)
    * [token](#token)
    * [deploy](#deploy)
1. [helm](#helm)
    * [deploy](#helm-deploy)
    * [grant](#grant)
    * [revoke](#revoke)

### eks

The `eks` subcommand of `kubergrunt` is used to setup the operator machine to interact with a Kubernetes cluster running
on EKS.

#### verify

This subcommand verifies that the specified EKS cluster is up and ready. An EKS cluster is considered ready when:

- The cluster status reaches ACTIVE state.
- The cluster Kubernetes API server endpoint responds to http requests.

When passing `--wait` to the command, this command will wait until the EKS cluster reaches the ready state, or it
times out. The timeout parameters are configurable with the `--max-retries` and `--sleep-between-retries` options, where
`--max-retries` specifies the number of times the command will try to verify a specific condition before giving up, and
`--sleep-between-retries` specifies the duration of time (e.g 10m = 10 minutes) to wait between each trial. So for
example, if you ran the command:

```bash
kubergrunt eks verify --eks-cluster-arn $EKS_CLUSTER_ARN --wait --max-retries 10 --sleep-between-retries 15s
```

and the cluster was not active yet, this command will query the AWS API up to 10 times, waiting 15 seconds inbetween
each try for a total of 150 seconds (2.5 minutes) before timing out.

Run `kubergrunt eks verify --help` to see all the available options.

#### configure

This subcommand will setup the installed `kubectl` with config contexts that will allow it to authenticate to a specified
EKS cluster. This binary is designed to be used as part of one of the modules in the package, although this binary
supports running as a standalone binary. For example, this binary might be used to setup a new operator machine to be
able to talk to an existing EKS cluster.

For example to setup a `kubectl` install on an operator machine to authenticate with EKS:

```bash
kubergrunt eks configure --eks-cluster-arn $EKS_CLUSTER_ARN
```

Run `kubergrunt eks configure --help` to see all the available options.

#### token

This subcommand is used by `kubectl` to retrieve an authentication token using the AWS API authenticated with IAM
credentials. This token is then used to authenticate to the Kubernetes cluster. This command embeds the
`aws-iam-authenticator` tool into `kubergrunt` so that operators don't have to install a separate tool to manage
authentication into Kubernetes.

The `configure` subcommand of `kubergrunt eks` assumes you will be using this method to authenticate with the Kubernetes
cluster provided by EKS. If you wish to use `aws-iam-authenticator` instead, replace the auth info clause of the `kubectl`
config context.

See [How do I authenticate kubectl to the EKS cluster?](../eks-cluster-control-plane/README.md#how-to-authenticate-kubectl) for more information on
authenticating `kubectl` with EKS.

#### deploy

This subcommand will initiate a rolling deployment of the current AMI config to the EC2 instances in your EKS cluster.
This command will not deploy or update an application deployed on your Kubernetes cluster (e.g `Deployment` resource,
`Pod` resource, etc). For that, refer to the [`k8s-service` module documentation](../k8s-service). Instead, this command
is for managing and deploying an update to the EC2 instances underlying your EKS cluster.

Terraform and AWS do not provide a way to automatically roll out a change to the Instances in an EKS Cluster. Due to
Terraform limitations (see [here for a
discussion](https://github.com/terraform-providers/terraform-provider-aws/issues/567)), there is currently no way to
implement this purely in Terraform code. Therefore, we've created this subcommand that can do a zero-downtime roll out
for you.

To deploy a change (such as rolling out a new AMI) to all EKS workers using this command:

1. Make sure the `cluster_max_size` is at least twice the size of `cluster_min_size`. The extra capacity will be used to
   deploy the updated instances.
1. Update the Terraform code with your changes (e.g. update the `cluster_instance_ami` variable to a new AMI).
1. Run `terraform apply`.
1. Run the command:

```bash
kubergrunt eks deploy --eks-cluster-arn CLUSTER_ARN --asg-name ASG_NAME
```

When you call the command, it will:

1. Double the desired capacity of the Auto Scaling Group that powers the EKS Cluster. This will launch new EKS workers
   with the new launch configuration.
1. Wait for the new nodes to be ready for Pod scheduling in Kubernetes. This includes waiting for the new nodes to be
   registered to any external load balancers managed by Kubernetes.
1. Drain the pods scheduled on the old EKS workers (using the equivalent of `kubectl drain`), so that they will be
   rescheduled on the new EKS workers.
1. Wait for all the pods to migrate off of the old EKS workers.
1. Set the desired capacity down to the original value and remove the old EKS workers from the ASG.

Note that to minimize service disruption from this command, your services should setup [a
PodDisruptionBudget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/), [a readiness
probe](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#define-readiness-probes)
that fails on container shutdown events, and implement graceful handling of SIGTERM in the container. You can learn more
about these features in [our blog post covering them](TODO).

Currently `kubergrunt` does not implement any checks for these resources to be implemented. However in the future, we
plan to bake in checks into the deployment command to verify that all services have a disruption budget set, and warn
the user of any services that do not have a check.


### helm

The `helm` subcommand of `kubergrunt` provides the ability to manage various Helm Server (Tiller) installs on your
Kubernetes cluster, in addition to setting up operator machines to authenticate with the designated Helm Server for the
operator.

**Note**: The `helm` subcommand requires the `helm` client to be installed on the operators' machine. Refer to the
[official docs](https://docs.helm.sh/) for instructions on installing the client.


#### (helm) deploy

This subcommand will install and setup the Helm Server on the designated Kubernetes cluster. In addition to providing a
basic helm server, this subcommand contains features such as:

- Provisioning and managing TLS certs for a particular Tiller install.
- Defaulting to use `Secrets` for Tiller release storage (as opposed to ConfigMaps).
- Specifying [ServiceAccounts](https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/) for a
  particular Tiller install.
- Tying certificate access to RBAC roles to harden access to the Tiller server.

Note that this command does not create `Namespaces` or `ServiceAccounts`, delegating that responsibility to other
systems (see [k8s-helm-server module](../k8s-helm-server) for example).

For example, to setup a basic install of helm in the Kubernetes namespace `tiller-world` with the `ServiceAccount`
`tiller`:

```bash
# Note that most of the arguments here are used to setup the Certificate Authority for TLS
kubergrunt helm deploy \
    --namespace tiller-world \
    --service-account tiller \
    --tls-common-name tiller \
    --tls-org Gruntwork \
    --tls-org-unit IT \
    --tls-city Phoenix \
    --tls-state AZ \
    --tls-country US
```

This will:

- Generate a new Certificate Authority keypair.
- Generate a new TLS certificate signed by the generated Certificate Authority keypair.
- Store the Certificate Authority private key in a new `Secret` in the `kube-system` namespace.
- Launch Tiller using the generated TLS certificate in the specified `Namespace` with the specified `ServiceAccount`.

#### grant

This subcommand will grant access to an installed helm server to a given RBAC role. This will:

- Download the corresponding CA keypair for the Tiller deployment from Kubernetes.
- Issue a new TLS certificate keypair using the CA keypair.
- Upload the new TLS certificate keypair to a new Secret in a new Namespace that only the granted RBAC role has access
  to. This access is readonly.
- Remove the local copies of the downloaded and generated certificates.

This command assumes that the authenticated entitiy running the command has enough permissions to access the generated
CA `Secret`.

For example, to grant access to a Tiller server deployed in the namespace `tiller-world` to the RBAC role `dev`:

```bash
kubergrunt helm grant --tiller-namespace tiller-world --rbac-role dev
```

#### revoke

This subcommand will revoke access to an installed helm server for a given RBAC role. This will:

- Download the corresponding CA keypair for the Tiller deployment from Kubernetes.
- Download the TLS certificate keypair for the RBAC role.
- Revoke the certificate in the CA.
- Update the CA certificate keypair in both the `Secret` and the installed Tiller server.
- Restart Tiller.

For example, to revoke access to a Tiller server deployed in the namespace `tiller-world` from the RBAC role `dev`:

```bash
kubergrunt helm revoke --tiller-namespace tiller-world --rbac-role dev
```
