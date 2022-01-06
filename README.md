[![Maintained by Gruntwork.io](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io/?ref=repo_kubergrunt)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/gruntwork-io/kubergrunt.svg?label=latest)](https://github.com/gruntwork-io/kubergrunt/releases/latest)

# kubergrunt

`kubergrunt` is a standalone go binary with a collection of commands that attempts to fill in the gaps between Terraform,
Helm, and Kubectl for managing a Kubernetes Cluster.

Some of the features of `kubergrunt` include:

* Configuring `kubectl` to authenticate with a given EKS cluster. Learn more about authenticating `kubectl` to EKS
  in the [our production deployment guide](https://gruntwork.io/guides/kubernetes/how-to-deploy-production-grade-kubernetes-cluster-aws/#authenticate).
* Managing Helm and associated TLS certificates on any Kubernetes cluster.
* Setting up Helm client with TLS certificates on any Kubernetes cluster.
* Generating TLS certificate key pairs and storing them as Kubernetes `Secrets` on any Kubernetes cluster.


## Installation

The binaries are all built as part of the CI pipeline on each release of the package, and is appended to the
corresponding release in the [Releases Page](/../../releases). You can download the corresponding binary for your
platform from the releases page.

Alternatively, you can install `kubergrunt` using the [Gruntwork
Installer](https://github.com/gruntwork-io/gruntwork-installer). For example, to install version `v0.5.13`:

```bash
gruntwork-install --binary-name "kubergrunt" --repo "https://github.com/gruntwork-io/kubergrunt" --tag "v0.5.13"
```

### 3rd party package managers

Note that third-party Kubergrunt packages may not be updated with the latest version, but are often close. Please check your version against the latest available on the [Releases Page](/../../releases).

Chocolatey (Windows):

```cmd
choco install kubergrunt
```

## Building from source

The main package is in `cmd`. To build the binary, you can run:

```
go build -o bin/kubergrunt ./cmd
```

If you need to set a version on the binary (so that `kubergrunt --version` works), you use `ldflags` to set the version
string on the compiled binary:

```
go build -o kubergrunt -ldflags "-X main.VERSION=v0.7.6 -extldflags '-static'" ./cmd
```


## Commands

The following commands are available as part of `kubergrunt`:

1. [eks](#eks)
    * [verify](#verify)
    * [configure](#configure)
    * [token](#token)
    * [oidc-thumbprint](#oidc-thumbprint)
    * [deploy](#deploy)
    * [sync-core-components](#sync-core-components)
    * [cleanup-security-group](#cleanup-security-group)
    * [schedule-coredns](#schedule-coredns)
    * [drain](#drain)
1. [k8s](#k8s)
    * [wait-for-ingress](#wait-for-ingress)
    * [kubectl](#kubectl)
1. [tls](#tls)
    * [gen](#gen)
1. [Deprecated commands](#deprecated-commands)
    * [helm](#helm)


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

Similar Commands:

- AWS CLI (`aws eks wait`): This command will wait until the EKS cluster reaches the ACTIVE state. Note that oftentimes
  the Kubernetes API endpoint has a delay in accepting traffic even after reaching the ACTIVE state. We have observed it
  take up to 1.5 minutes after the cluster becomes ACTIVE before we can have a valid TCP connection with the Kubernetes
  API endpoint.

#### configure

This subcommand will setup the installed `kubectl` with config contexts that will allow it to authenticate to a
specified EKS cluster by leveraging the `kubergrunt eks token` command. This binary is designed to be used as part of
one of the modules in the package, although this binary supports running as a standalone binary. For example, this
binary might be used to setup a new operator machine to be able to talk to an existing EKS cluster.

For example to setup a `kubectl` install on an operator machine to authenticate with EKS:

```bash
kubergrunt eks configure --eks-cluster-arn $EKS_CLUSTER_ARN
```

Run `kubergrunt eks configure --help` to see all the available options.

Similar Commands:

- AWS CLI (`aws eks update-kubeconfig`): This command will configure `kubeconfig` in a similar manner. Instead of using
  `kubergrunt eks token`, this version will use the `get-token` subcommand built into the AWS CLI.

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

Similar Commands:

- AWS CLI (`aws eks get-token`): This command will do the same thing, but does not provide any specific optimizations
  for terraform.
- Terraform [`aws_eks_cluster_auth`](https://www.terraform.io/docs/providers/aws/d/eks_cluster_auth.html) data source:
  This data source can be used to retrieve a temporary auth token for EKS in Terraform. This can only be used in
  Terraform.
- [`aws-iam-authenticator`](https://github.com/kubernetes-sigs/aws-iam-authenticator): This is a standalone binary that
  can be used to fetch a temporary auth token.

#### oidc-thumbprint

This subcommand will take the EKS OIDC Issuer URL and retrieve the root CA thumbprint. This is used to set the trust
relation for any certificates signed by that CA for the issuer domain. This is necessary to setup the OIDC provider,
which is used for the IAM Roles for Service Accounts feature of EKS.

You can read more about the general procedure for retrieving the root CA thumbprint of an OIDC Provider in [the official
documentation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc_verify-thumbprint.html).

To retrieve the thumbprint, call the command with the issuer URL:

```bash
kubergrunt eks oidc-thumbprint --issuer-url $ISSUER_URL
```

This will output the thumbprint to stdout in JSON format, with the key `thumbprint`.

Run `kubergrunt eks oidc-thumbprint --help` to see all the available options.

Similar Commands:

- You can use `openssl` to retrieve the thumbprint as described by [the official
  documentation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc_verify-thumbprint.html).
- `eksctl` provides routines for directly configuring the OIDC provider so you don't need to retrieve the thumbprint.

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
1. Cordon the old instances in the ASG so that they won't schedule new Pods.
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

**`eks deploy` recovery file**

Due to the nature of rolling update, the `deploy` subcommand performs multiple sequential actions that 
depend on success of the previous operations. To mitigate intermittent failures, the `deploy` subcommand creates a
recovery file in the working directory for storing current deploy state. The recovery file is updated after 
each stage and if the `deploy` subcommand fails for some reason, execution resumes from the last successful state.
The existing recovery file can also be ignored with the `--ignore-recovery-file` flag. In this case the recovery 
file will be re-initialized.

#### sync-core-components

This subcommand will sync the core components of an EKS cluster to match the deployed Kubernetes version by following
the steps listed [in the official documentation](https://docs.aws.amazon.com/eks/latest/userguide/update-cluster.html).

The core components managed by this command are:

- kube-proxy
- Amazon VPC CNI plug-in
- CoreDNS

By default, this command will rotate the images without waiting for the Pods to be redeployed. You can use the `--wait`
option to force the command to wait until all the Pods have been replaced.

Example:

```bash
kubergrunt eks sync-core-components --eks-cluster-arn EKS_CLUSTER_ARN
```

#### cleanup-security-group
This subcommand cleans up the leftover AWS-managed security groups that are associated with an EKS cluster you intend
to destroy. It accepts
- `--eks-cluster-arn`: the ARN of the EKS cluster
- `--security-group-id`: a known security group ID associated with the EKS cluster
- `--vpc-id`: the VPC ID where the cluster is located

It also looks for other security groups associated with the EKS cluster, such as the security group created by the AWS
Load Balancer Controller. To safely delete these resources, it detaches and deletes any associated AWS Elastic Network
Interfaces.

Example:

```bash
kubergrunt eks cleanup-security-group --eks-cluster-arn EKS_CLUSTER_ARN --security-group-id SECURITY_GROUP_ID \
--vpc-id VPC_ID
```

#### schedule-coredns
This subcommand can be used to toggle the CoreDNS service between scheduling on Fargate and EC2 worker types. During
the creation of an EKS cluster that uses Fargate, `schedule-coredns fargate` will annotate the deployment so that
CoreDNS can find and allow EKS to use Fargate nodes. To switch back to EC2, you can run `schedule-coredns ec2` to
reset the annotations such that EC2 nodes can be found by CoreDNS.

This command is useful when creating Fargate only EKS clusters. By default, EKS will schedule the `coredns` service
assuming EC2 workers. You can use this command to force the service to run on Fargate.

You can also use this command in `local-exec` provisioners on an `aws_eks_fargate_profile` resource so you can
schedule the CoreDNS service after creating the profile, and revert back when destroying the profile.

Currently `fargate` and `ec2` are the only subcommands that `schedule-coredns` accepts.

Examples:

```bash
kubergrunt eks schedule-coredns fargate --eks-cluster-name EKS_CLUSTER_NAME --fargate-profile-arn FARGATE_PROFILE_ARN
```

```bash
kubergrunt eks schedule-coredns ec2 --eks-cluster-name EKS_CLUSTER_NAME --fargate-profile-arn FARGATE_PROFILE_ARN
```

#### drain

This subcommand can be used to drain Pods from the instances in the provided Auto Scaling Groups. This can be used to
gracefully retire existing Auto Scaling Groups by ensuring the Pods are evicted in a manner that respects disruption
budgets.

You can read more about the drain operation in [the official
documentation](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/).

To drain the Auto Scaling Group `my-asg` in the region `us-east-2`:

```bash
kubergrunt eks drain --asg-name my-asg --region us-east-2
```

You can drain multiple ASGs by providing the `--name` option multiple times:

```bash
kubergrunt eks drain --asg-name my-asg-a --name my-asg-b --name my-asg-c --region us-east-2
```


### k8s

The `k8s` subcommand of `kubergrunt` includes commands that directly interact with the Kubernetes resources.

#### wait-for-ingress

This subcommand waits for the Ingress endpoint to be provisioned. This will monitor the Ingress resource, continuously
checking until the endpoint is allocated to the Ingress resource or times out. By default, this will try for 5 minutes
(max retries 60 and time betweeen sleep of 5 seconds).

You can configure the timeout settings using the --max-retries and --sleep-between-retries CLI args. This will check for
--max-retries times, sleeping for --sleep-between-retries inbetween tries.

For example, if you ran the command:

```bash
kubergrunt k8s wait-for-ingress \
    --ingress-name $INGRESS_NAME \
    --namespace $NAMESPACE \
    --max-retries 10 \
    --sleep-between-retries 15s
```

this command will query the Kubernetes API to check the `Ingress` resource up to 10 times, waiting for 15 seconds
inbetween each try for a total of 150 seconds (2.5 minutes) before timing out.

Run `kubergrunt k8s wait-for-ingress --help` to see all the available options.

#### kubectl

This subcommand will call out to kubectl with a temporary file that acts as the kubeconfig, set up with the parameters
`--kubectl-server-endpoint`, `--kubectl-certificate-authority`, `--kubectl-token`. Unlike using kubectl directly, this
command allows you to pass in the base64 encoded certificate authority data directly as opposed to as a file.

To forward args to kubectl, pass all the args you wish to forward after a `--`. For example, the following command runs
`kubectl get pods -n kube-system`:

```
kubergrunt k8s kubectl \
  --kubectl-server-endpoint $SERVER_ENDPOINT \
  --kubectl-certificate-authority $SERVER_CA \
  --kubectl-token $TOKEN \
  -- get pods -n kube-system
```

Run `kubergrunt k8s kubectl --help` to see all the available options.


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


### Deprecated commands

#### helm

The `helm` subcommand contained utilities for managing Helm v2, and is not necessary for Helm v3. This subcommand was
removed as of `kubergrunt` version `v0.6.0` with Helm v2 reaching end of life.




## Who maintains this project?

`kubergrunt` is maintained by [Gruntwork](http://www.gruntwork.io/). If you are looking for help or commercial support,
send an email to [support@gruntwork.io](mailto:support@gruntwork.io?Subject=kubergrunt).

Gruntwork can help with:

* Setup, customization, and support for this project.
* Modules and submodules for other types of infrastructure in major cloud providers, such as VPCs, Docker clusters,
  databases, and continuous integration.
* Modules and Submodules that meet compliance requirements, such as HIPAA.
* Consulting & Training on AWS, GCP, Terraform, and DevOps.


## How do I contribute?

Contributions are very welcome! Check out the [Contribution Guidelines](/CONTRIBUTING.md) for instructions.


## How is this project versioned?

This project follows the principles of [Semantic Versioning](http://semver.org/). You can find each new release, along
with the changelog, in the [Releases Page](../../releases).

During initial development, the major version will be 0 (e.g., `0.x.y`), which indicates the code does not yet have a
stable API. Once we hit `1.0.0`, we will make every effort to maintain a backwards compatible API and use the MAJOR,
MINOR, and PATCH versions on each release to indicate any incompatibilities.


## License

Please see [LICENSE](/LICENSE) and [NOTICE](/NOTICE) for how the code in this repo is licensed.

Copyright &copy; 2020 Gruntwork, Inc.
