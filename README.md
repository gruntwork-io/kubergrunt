[![Maintained by Gruntwork.io](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io/?ref=repo_kubergrunt)

# kubergrunt

`kubergrunt` is a collection of scripts embedded into a go binary that attempts to fill in the gaps between Terraform,
Helm, and Kubectl for managing a Kubernetes Cluster.

Some of the features of `kubergrunt` include:

* configuring `kubectl` to authenticate with a given EKS cluster. Learn more about authenticating `kubectl` to EKS
  in the [eks-cluster module README](../eks-cluster/README.md#how-to-authenticate-kubectl).
* managing Helm and associated TLS certificates.
* setting up Helm client with TLS certificates.


## Installation

The binaries are all built as part of the CI pipeline on each release of the package, and is appended to the
corresponding release in the [Releases Page](/../../releases). You can download the corresponding binary for your
platform from the releases page.

Alternatively, you can install `kubergrunt` using the [Gruntwork
Installer](https://github.com/gruntwork-io/gruntwork-installer). For example, to install version `v0.3.6`:

```bash
gruntwork-install --binary-name "kubergrunt" --repo "https://github.com/gruntwork-io/kubergrunt" --tag "v0.3.6"
```


## Building from source

The main package is in `cmd`. To build the binary, you can run:

```
go build -o kubergrunt ./cmd
```


## Commands

The following commands are available as part of `kubergrunt`:

1. [eks](#eks)
    * [verify](#verify)
    * [configure](#configure)
    * [token](#token)
    * [deploy](#deploy)
1. [helm](#helm)
    * [deploy](#helm-deploy)
    * [undeploy](#undeploy)
    * [configure](#helm-configure)
    * [grant](#grant)
    <!-- not implemented * [revoke](#revoke) -->
1. [tls](#tls)
    * [gen](#gen)

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

This subcommand also supports outputting the token in a format that is consumable by terraform as an [external data
source](https://www.terraform.io/docs/providers/external/data_source.html) when you pass in the `--as-tf-data` CLI arg.
You can then pass the token directly into the `kubernetes` provider configuration. For example:

```hcl
# NOTE: Terraform does not allow you to interpolate resources in a provider config. We work around this by using the
# template_file data source as a means to compute the resource interpolations.
provider "kubernetes" {
  load_config_file       = false
  host                   = "${data.template_file.kubernetes_cluster_endpoint.rendered}"
  cluster_ca_certificate = "${base64decode(data.template_file.kubernetes_cluster_ca.rendered)}"
  token                  = "${lookup(data.external.kubernetes_token.result, "token_data")}"
}

data "template_file" "kubernetes_cluster_endpoint" {
  template = "${module.eks_cluster.eks_cluster_endpoint}"
}

data "template_file" "kubernetes_cluster_ca" {
  template = "${module.eks_cluster.eks_cluster_certificate_authority}"
}

data "external" "kubernetes_token" {
  program = ["kubergrunt", "--loglevel", "error", "eks", "token", "--as-tf-data", "--cluster-id", "${module.eks_cluster.eks_cluster_name}"]
}
```

This will configure the `kubernetes` provider in Terraform without setting up kubeconfig, allowing you to do everything
in Terraform without side effects to your local machine.


#### deploy

This subcommand will initiate a rolling deployment of the current AMI config to the EC2 instances in your EKS cluster.
This command will not deploy or update an application deployed on your Kubernetes cluster (e.g `Deployment` resource,
`Pod` resource, etc). We provide helm charts that you can use to deploy your applications on to a Kubernetes cluster.
See our [`helm-kubernetes-services` repo](https://github.com/gruntwork-io/helm-kubernetes-services/) for more info.
Instead, this command is for managing and deploying an update to the EC2 instances underlying your EKS cluster.

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
kubergrunt eks deploy --region REGION --asg-name ASG_NAME
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
about these features in [our blog post series covering
them](https://blog.gruntwork.io/zero-downtime-server-updates-for-your-kubernetes-cluster-902009df5b33).

Currently `kubergrunt` does not implement any checks for these resources to be implemented. However in the future, we
plan to bake in checks into the deployment command to verify that all services have a disruption budget set, and warn
the user of any services that do not have a check.


### helm

The `helm` subcommand of `kubergrunt` provides the ability to manage various Helm Server (Tiller) installs on your
Kubernetes cluster, in addition to setting up operator machines to authenticate with the designated Helm Server for the
operator, while following the security best practices from the community.

If you are not familiar with Helm, be sure to check out [our guide](/HELM_GUIDE.md).

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

**Note**: This command does not create `Namespaces` or `ServiceAccounts`, delegating that responsibility to other
systems.

<!-- TODO: https://github.com/gruntwork-io/kubergrunt/issues/15 -->

For example, to setup a basic install of helm in the Kubernetes namespace `tiller-world` that manages resources in the
Kubernetes namespace `dev` with the service account `tiller`:

```bash
# Note that most of the arguments here are used to setup the Certificate Authority for TLS
kubergrunt helm deploy \
    --tiller-namespace tiller-world \
    --resource-namespace dev \
    --service-account tiller \
    --tls-common-name tiller \
    --tls-org Gruntwork \
    --tls-org-unit IT \
    --tls-city Phoenix \
    --tls-state AZ \
    --tls-country US \
    --rbac-group admin \
    --client-tls-common-name admin \
    --client-tls-org Gruntwork
```

This will:

- Generate a new Certificate Authority keypair.
- Generate a new TLS certificate signed by the generated Certificate Authority keypair.
- Store the Certificate Authority private key in a new `Secret` in the `kube-system` namespace.
- Launch Tiller using the generated TLS certificate in the specified `Namespace` with the specified `ServiceAccount`.

This command will also grant access to an RBAC entity and configure the local helm client to use that using one of `--rbac-user`, `--rbac-group`, `--rbac-service-account` options.

This command should be run by a **cluster administrator** to deploy a new Tiller instance that can be used by their
users to deploy resources using `helm`.

#### undeploy

This subcommand will uninstall the Helm Server from the designated Kubernetes cluster and delete all Secrets related to
the installed Helm Server. This subcommand relies on the `helm reset` command, which requires a helm client that can
access the Helm Server pod being uninstalled. See the [`configure`](#helm-configure) subcommand for more details on
setting up your helm client.

**Note**: By default, this will not uninstall the Helm server if there are any deployed releases. You can force removal
of the server using the `--force` option, but this will not delete any releases. Given the destructive nature of such an
operation, we intentionally added a second option for removing the releases (`--undeploy-releases`).

For example, if you had a deployed a Helm server into the namespace `dev` using the [`deploy`](#helm-deploy) command and
wanted to uninstall it:

```bash
# The helm-home option should point to a helm home directory that has been configured for the Helm server being
# undeployed.
kubergrunt helm undeploy --helm-home $HOME/.helm
```

This command should be run by a **cluster administrator** to undeploy a deployed Tiller instance, along with all the
`Secrets` containing the TLS certificate key pairs.

#### (helm) configure

This subcommand will setup the installed `helm` client to be able to access the specified Helm server. Specifically,
this will:

- Download the client TLS certificate key pair generated with the [`grant`](#grant) command.
- Install the TLS certificate key pair in the helm home directory.
- Install an environment file that sets up environment variables to target the specific helm server. This environment
  file needs to be loaded before issuing any commands, at it sets the necessary environment variables to signal to the
  helm client which helm server to use. The environment variables it sets are:
  - `HELM_HOME`: The helm client home directory where the TLS certs are located.
  - `TILLER_NAMESPACE`: The namespace where the helm server is installed.
  - `HELM_TLS_VERIFY`: This will be set to true to enable TLS verification.
  - `HELM_TLS_ENABLE`: This will be set to true to enable TLS authentication.

You can also optionally set the current kubectl context to set the default namespace to be compatible with this Tiller
install.

Afterwards, you can source the environment file to setup your shell to access the proper helm client.

For example, if you want to setup helm to target a Tiller installed in the namespace `dev-tiller` to manage resources in
the namespace `dev` with the default helm home directory:

```bash
# This is for linux
# Setup helm
kubergrunt helm configure --tiller-namespace dev-tiller --resource-namespace dev --rbac-user me
# Source the environment file
source $HOME/.helm/env
# Verify connection. This should display info about both the client and server.
helm version
```

See the command help for all the available options: `kubergrunt helm configure --help`.

This command should be run by a **helm user** to setup their local `helm` client to access a deployed Tiller instance.
Note that the user needs to have already been granted access via the `kubergrunt helm grant` command.

#### grant

This subcommand will grant access to an installed helm server to a given RBAC entity (`User`, `Group`, or
`ServiceAccount`). This will:

- Download the corresponding CA keypair for the Tiller deployment from Kubernetes.
- Issue a new TLS certificate keypair using the CA keypair.
- Upload the new TLS certificate keypair to a new Secret in a new Namespace that only the granted RBAC entity has access
  to. This access is readonly.
- Remove the local copies of the downloaded and generated certificates.

This command assumes that the authenticated entitiy running the command has enough permissions to access the generated
CA `Secret`.

For example, to grant access to a Tiller server deployed in the namespace `tiller-world` to the RBAC group `developers`:

```bash
kubergrunt helm grant \
    --tls-common-name developers \
    --tls-org YourCo \
    --tiller-namespace tiller-world \
    --rbac-group developers
```

See the command help for all the available options: `kubergrunt helm grant --help`.

This command should be run by a **cluster administrator** to grant a user, group, or pod access to the deployed Tiller
instance. The user or group should be an RBAC entity (RBAC user or RBAC group). The pod should gain access via the
mounted `ServiceAccount`.

<!-- Not implemented

#### revoke

This subcommand will revoke access to an installed helm server for a given RBAC role. This will:

- Download the corresponding CA keypair for the Tiller deployment from Kubernetes.
- Download the TLS certificate keypair for the RBAC role.
- Revoke the certificate in the CA.
- Update the CA certificate keypair in both the `Secret` and the installed Tiller server.
- Restart Tiller.

For example, to revoke access to a Tiller server deployed in the namespace `tiller-world` from the RBAC role `dev`:

```bash
kubergrunt helm revoke --tiller-namespace tiller-world --rbac-user dev
```

See the command help for all the available options: `kubergrunt helm revoke --help`.

-->

### tls

The `tls` subcommand of `kubergrunt` is used to manage TLS certificate key pairs as Kubernetes Secrets.

#### gen

This subcommand will generate new TLS certificate key pairs based on the provided configuration arguments. Once the
certificates are generated, they will be stored on your targeted Kubernetes cluster as
[Secrets](https://kubernetes.io/docs/concepts/configuration/secret/). This supports features such as:

- Generating a new CA key pair and storing the generated key pair in your Kubernetes cluster.
- Issuing a new signed TLS certificate key pair using an existing CA stored in your Kubernetes cluster.
- Replacing the stored certificate key pair in your Kubernetes cluster with a newly generated one.
- Controlling which Namespace the Secrets are stored in.

For example, to generate a new CA key pair, issue a TLS certificate key pair, storing each of those as the Secrets
`ca-keypair` and `tls-keypair` respectively:

```bash
# Generate the CA key pair
kubergrunt tls gen \
    --namespace kube-system \
    --secret-name ca-keypair \
    --ca \
    --tls-common-name kiam-ca \
    --tls-org Gruntwork \
    --tls-org-unit IT \
    --tls-city Phoenix \
    --tls-state AZ \
    --tls-country US \
    --secret-annotation "gruntwork.io/version=v1"
# Generate a signed TLS key pair using the previously created CA
kubergrunt tls gen \
    --namespace kube-system \
    --secret-name tls-keypair \
    --ca-secret-name ca-keypair \
    --tls-common-name kiam-server \
    --tls-org Gruntwork \
    --tls-org-unit IT \
    --tls-city Phoenix \
    --tls-state AZ \
    --tls-country US \
    --secret-annotation "gruntwork.io/version=v1"
```

The first command will generate a CA key pair and store it as the Secret `ca-keypair`. The `--ca` argument signals to
`kubergrunt` that the TLS certificate is for a CA.

The second command uses the generated CA key pair to issue a new TLS key pair. The `--ca-secret-name` signals
`kubergrunt` to use the CA key pair stored in the Kubernetes Secret `ca-keypair`.

This command should be run by a **cluster administrator** to ensure access to the Secrets are tightly controlled.

See the command help for all the available options: `kubergrunt tls gen --help`.


## Who maintains this project?

`kubergrunt` is maintained by [Gruntwork](http://www.gruntwork.io/). If you are looking for help or commercial support,
send an email to [support@gruntwork.io](mailto:support@gruntwork.io?Subject=kubergrunt).

Gruntwork can help with:

* Setup, customization, and support for this project.
* Modules and submodules for other types of infrastructure, such as VPCs, Docker clusters, databases, and continuous
  integration.
* Modules and Submodules that meet compliance requirements, such as HIPAA.
* Consulting & Training on AWS, Terraform, and DevOps.


## How do I contribute?

Contributions are very welcome! Check out the [Contribution Guidelines](/CONTRIBUTING.md) for instructions.


## How is this project versioned?

This project follows the principles of [Semantic Versioning](http://semver.org/). You can find each new release, along
with the changelog, in the [Releases Page](../../releases).

During initial development, the major version will be 0 (e.g., `0.x.y`), which indicates the code does not yet have a
stable API. Once we hit `1.0.0`, we will make every effort to maintain a backwards compatible API and use the MAJOR,
MINOR, and PATCH versions on each release to indicate any incompatibilities.


## License

Please see [LICENSE](/LICENSE) for how the code in this repo is licensed.

Copyright &copy; 2019 Gruntwork, Inc.
