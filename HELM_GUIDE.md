# What is Helm?

[Helm](https://helm.sh/) is a package and module manager for Kubernetes that allows you to define, install, and manage
Kubernetes applications as reusable packages called Charts. Helm provides support for official charts in their
repository that contains various applications such as Jenkins, MySQL, and Consul to name a few. Gruntwork uses Helm
under the hood for the Kubernetes modules in this package.

In the current version of Helm, there are two components: the Helm Client, and the Helm Server (Tiller).

**Note**: There are plans in the community to reduce this down to just the client starting with v3, but there is no
release date yet.


## What is the Helm Client?

The Helm client is a command line utility that provides a way to interact with Tiller. It is the primary interface to
installing and managing Charts as releases in the Helm ecosystem. In addition to providing operational interfaces (e.g
install, upgrade, list, etc), the client also provides utilities to support local development of Charts in the form of a
scaffolding command and repository management (e.g uploading a Chart).


## What is Tiller (the Helm Server)?

Tiller is a component of Helm that runs inside the Kubernetes cluster. Tiller is what provides the functionality to
apply the Kubernetes resource descriptions to the Kubernetes cluster. When you install a release, the `helm` client
essentially packages up the values and charts as a release, which is submitted to Tiller. Tiller will then generate
Kubernetes YAML files from the packaged release, and then apply the generated Kubernetes YAML file from the charts on
the cluster.

### Security Model of Tiller (the server component of Helm)

By design, Tiller is the responsible entity in Helm to apply the Kubernetes config against the cluster. What this means
is that Tiller needs enough permissions in the Kubernetes cluster to be able to do anything requested by a Helm chart.
These permissions are granted to Tiller through the use of `ServiceAccounts`, which are credentials that a Kubernetes
pod can inherit when making calls to the Kubernetes server.

Currently there is no way for Tiller to be able to inherit the permissions of the calling entity (e.g the user accessing
the server via the Helm client). In practice, this means that any user who has access to the Tiller server is able to
gain the same permissions granted to that server even though their RBAC roles may be more restrictive. In other words,
if Tiller has admin permissions (the default), then all users that have access to it via helm effectively has admin
permissions on the Kubernetes cluster.

Tiller provides two mechanisms to handle the permissions given their design:

- Using `ServiceAccounts` to restrict what Tiller can do
- Using TLS based authentication to restrict who has access to the Tiller server

You can read more about the security model of Tiller in [their official docs](https://docs.helm.sh/using_helm/#securing-your-helm-installation).

#### Client Authentication

This module installs Tiller with TLS verification turned on. If you are unfamiliar with TLS/SSL, we recommend reading
[this background](https://github.com/hashicorp/terraform-aws-vault/tree/master/modules/private-tls-cert#background)
document describing how it works before continuing.

With this feature, Tiller will validate client side TLS certificates provided as part of the API call to ensure the
client has access. Likewise, the client will also validate the TLS certificates provided by Tiller. In this way, both
the client and the server can trust each other as authorized entities.

To achieve this, we will need to generate a Certificate Authority (CA) that can be used to issue and validate
certificates. This CA will be shared between the server and the client to validate each others' certificates.

Then, using the generated CA, we will issue at least two sets of signed certificates:

- A certificate for Tiller that identifies it.
- A certificate for the Helm client that identifies it.

We recommend that you issue a certificate for each unique `helm` client (and therefore each user of helm). This makes it
easier to manage access for team changes (e.g when someone leaves the team), as well as compliance requirements (e.g
access logs that uniquely identifies individuals).

Finally, both Tiller and the Helm client need to be setup to utilize the issued certificates.

To summarize, assuming a single client, in this model we have three sets of TLS key pairs in play:

- Key pair for the CA to issue new certificate key pairs.
- Key pair to identify Tiller.
- Key pair to identify the client.

#### Service Account

Tiller relies on `ServiceAccounts` and the associated RBAC roles to properly restrict what Helm Charts can do. The RBAC
system in Kubernetes allows the operator to define fine grained permissions on what an individual or system can do in
the cluster. By using RBAC, you can restrict Tiller installs to only manage resources in particular namespaces, or even
restrict what resources Tiller can manage.

At a minimum, each Tiller server should be deployed in its own namespace separate from the namespace to manage
resources, and restricted to only be able to access those namespaces. This can be done by creating a `ServiceAccount`
limited to that namespace, with permissions granted to access the resource namespace:

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dev-service-account
  namespace: dev-tiller
  labels:
    app: tiller
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  namespace: dev-tiller
  name: dev-tiller-all
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: dev-role-dev-tiller-all-members
  namespace: dev-tiller
subjects:
  - kind: Group
    name: system:serviceaccounts:dev-service-account
roleRef:
  kind: Role
  name: dev-tiller-all
  apiGroup: "rbac.authorization.k8s.io"
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  namespace: dev
  name: dev-all
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: dev-role-dev-all-members
  namespace: dev
subjects:
  - kind: Group
    name: system:serviceaccounts:dev-service-account
roleRef:
  kind: Role
  name: dev-all
  apiGroup: "rbac.authorization.k8s.io"
```

This resource configuration creates a new `ServiceAccount` named `dev-service-account`. This resource then creates two
RBAC roles: one to grant admin permissions to the `dev` namespace (`dev-all`) and one to grant admin permissions to the
`dev-tiller` namespace (`dev-tiller-all`). Finally, the configuration specifies `RoleBinding` resources to bind the new
roles to the `ServiceAccount`.

In general, you will want to restrict Tiller to have the same permissions as the users that are granted access to it. In
this way, you can avoid having your users "escape" RBAC by having more permissions through Tiller. The key insight here
is that clients with access to Tiller have roughly the same permissions as that granted to the Tiller pod.


#### Namespaces: Tiller Namespace vs Resource Namespace

We recommend provisioning Tiller in its own namespace, separate from the namespace where the resources will ultimately
be deployed. The primary reason for doing this is so that you can lock down access to Tiller. Typically it is
challenging to implement RBAC controls to prevent access to specific resources within a `Namespace`, and still be
functional. For example, it is challenging to come up with rules to allow listing pods in a namespace while denying
access to a specific pod. This is because the `list` action in RBAC automatically pulls in the details included in a
`get` action, yet you can only limit access to specific resources on a `get` action in RBAC.

In practice, you will want to grant your users enough permissions in the resource namespace so that your users can
access the resources being deployed to perform their daily actions. This might include listing pods, setting up port
forwards to services, or even listing secrets in the namespace. If you share the namespace between where Tiller is
deployed and where the resources will be deployed, it is easy to accidentally set enough permissions to your users to be
able to access Tiller's resources. This includes the `ServiceAccount` credentials and the server side TLS certificates
that the Tiller pod uses.

This is why we recommend specifying a different namespace to deploy Tiller from where the resources are deployed.

Note: The exception to this is when you want to use `helm` to manage admin level resources (e.g deploying [the Kubernetes
Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)). In this case, the Tiller
deployment will manage resources in the `kube-system` namespace, which is the admin level namespace of the cluster. For
this namespace, you will want your Tiller instance to also be in the `kube-system` namespace so that it shares all the
locked down access properties of that namespace.


## Threat model

As discussed above, Tiller does not provide a way to inherit permissions of the calling entity. This means that you can
not rely on your RBAC configurations of the user to restrict what they can deploy through helm.

To illustrate this, consider the following example:

Suppose that you have installed Tiller naively using the defaults, but configured it to use TLS authentication based on
the documentation. You configured Tiller with a pair of TLS certificates that you shared with your developers so that
they can access the server to use Helm. Note that a default Tiller install has admin level permissions on the Kubernetes
cluster (a `ServiceAccount` with permissions in the `kube-system` namespace) to be able to do almost anything in
the cluster.

Suppose further that you originally wanted to restrict your developers from being able to access `Secrets` created in a
particular namespace called `secret`. You had originally implemented RBAC roles that prevented accessing this namespace
for Kubernetes users in the group `developers`. All your developers have credentials that map to the `developers` group
in Kubernetes. You have verified that they can not access this namespace using `kubectl`.

In this scenario, Tiller poses a security risk to your team. Specifically, because Tiller has been deployed with admin
level permissions in the cluster, and because your developers have access to Tiller via Helm, your developers are now
able to deploy Helm charts that perform certain actions in the `secret` namespace despite not being able to directly
access it. This is because Tiller is the entity that is actually applying the configs from the rendered Helm charts.
This means that the configs are being applied with Tiller's credentials as opposed to the developer credentials. As
such, your developers can deploy a pod that has access to `Secrets` created in the `secret` namespace and potentially
read the information stored there by having the pod send the data to some arbitrary endpoint (or another pod in the
system), bypassing the RBAC restrictions you have created.

Therefore, it is important that the Tiller servers that your team has access to is deployed with `ServiceAccounts` that
maintain a strict subset of the permissions granted to the users. This means that you potentially need one Tiller server
per RBAC role in your Kubernetes cluster.

It is also important to lock down access to the TLS certs used to authenticate against the various Tiller servers
deployed in your Kubernetes environment, so that your users can only access the Tiller deployment that maps to their
permissions. Each of the key pairs have varying degrees of severity when compromised:

- If you possess the CA key pair, then you can issue additional certificates to pose as **both** Tiller and the client.
  This means that you can:
    - Trick the client in installing charts with secrets to a compromised Tiller server that could forward it to a
      malicious entity.
    - Trick Tiller into believing a client has access and install malicious Pods on the Kubernetes cluster.

- If you possess the Tiller key pair, then you can pose as Tiller to trick a client in installing charts on a compromised
  Tiller server.
- If you possess the client key pair, then you can pose as the client and install malicious charts on the Kubernetes
  cluster.


## Kubergrunt Approach

To summarize the best practices for a secure Tiller deployment, one should:

- [Deploy Tiller into its own `Namespace`, separately from the namespace where it will allow helm charts to deploy
  into.](#deploying-tiller-into-its-own-namespace)
- [Enable RBAC and restrict what Tiller can do by creating its own
  `ServiceAccount`.](#enable-rbac-and-specify-tiller-serviceaccount)
- [Ensure Tiller stores its metadata in `Secrets`.](#ensure-tiller-stores-its-metadata-using-secrets)
- [Enable TLS verification in the server so that only authorized clients can access
  it.](#enable-tls-verification-in-the-server)
- [Restrict client and server access to the Tiller
  `Namespace`.](#restrict-client-and-server-access-to-the-tiller-namespace)
- [Enable TLS verification in the client so that it will only access servers that it
  trusts.](#enable-tls-verification-in-the-client)

`kubergrunt` includes various commands to help enable those best practices. You can read more about each individual
commands in [the command docs](/README.md#helm).

### Deploying Tiller into its own Namespace

`kubergrunt helm deploy` can be used to deploy a Tiller instance into your Kubernetes cluster. This command forces the
operator to pass in the Tiller namespace (the `Namespace` where Tiller will be deployed into), which is separate from
the resource namespace (the `Namespace`) where Tiller will be allowed to deploy resources into.

By forcing the operator to specify the namespaces directly as part of deployment, it ensures that the decision on where
Tiller is deployed and where it will manage resources are made explicitly as opposed to following defaults that may not
be the best for your cluster.

This contrasts with the defaults set by the `helm init` command provided by the `helm` CLI, which will default to
deploying in the `kube-system` namespace, where it has access to administrative systems of the Kubernetes cluster. While
this is a reasonable default for getting up and running, it is important to consider all the security implications of
having such a deployment in your cluster. This decision to deploy into `kube-system` should be made consciously, which
is what `kubergrunt helm deploy` hopes to accomplish.

### Enable RBAC and Specify Tiller ServiceAccount

Like the namespaces, `kubergrunt helm deploy` also requires specifying a `ServiceAccount` when deploying Tiller. This
`ServiceAccount` will be used by the Tiller Pod when it authenticates against the Kubernetes API.

By forcing the operator to specify the `ServiceAccount` directly as part of deployment, it ensures that a conscious
decision is made about what permissions are granted to the Tiller pod. For example, if you are going to give Tiller
cluster admin level permissions, then you should be conscious about that decision and explicitly pass that in as a
command line parameter.

This contrasts with the defaults set by the `helm init` command provided by the `helm` CLI, which will default to the
default `ServiceAccount` in the chosen namespace, which often times has admin level privileges in that namespace. This
is a reasonable default for getting up and running, but it is important to consider all the security implications of
having your Tiller pod be able to do anything in the chosen namespace.

As [mentioned in this guide](#service-account), you will want to ensure that the deployed Tiller is only granted the
same permissions as the minimum permissions granted to your clients accessing it. Otherwise, you risk having your users
"escape" the RBAC permissions imposed on them.

### Ensure Tiller Stores its Metadata using Secrets

`kubergrunt helm deploy` will deploy Tiller with overrides to configure the Pod to use `Secrets` as its metadata store.
The command does not expose a way to turn this off.

### Enable TLS verification in the Server

`kubergrunt helm deploy` will deploy Tiller with TLS authentication turned on (see section [Client
Authentication](#client-authentication) for more details on TLS authentication in Tiller). The command does not expose a
way to turn this off.

As a part of this, the `kubergrunt` command will generate two new TLS certificate key pairs:

- a key pair that can be used as the CA for verifying the server and the clients.
- a key pair that can be used to identify the server.

The private key for the generated CA key pair will be stored in the Kubernetes cluster as a `Secret` in the
`kube-system` namespace. Doing so allows future operators to generate new certificate key pairs to grant additional
clients access to the deployed Tiller instance, while protecting access to the CA key pair using the native RBAC system
of the cluster. By placing the CA key pair in the `kube-system` namespace, it makes it difficult to accidentally grant
access to it outside your group of trusted operators (the cluster admins).

The certificate key pair that Tiller will use will be placed into a `Secret` in the Tiller namespace so that the Tiller
pod may access it. This TLS certificate key pair will then be used as part of the TLS protocol so that clients can use
it to verify if they are talking to the correct Tiller instance.

At this point, your `helm` clients now need TLS certificate key pairs signed by the deployed CA to be able to access the
Tiller deployment. This ensures that a malicious pod in the Kubernetes cluster won't be able to inadvertently reach the
Tiller instance to deploy additional malicious pods that steal credentials or do damage to your cluster. This also means
that you will need to generate additional certificate key pairs for each of your clients.

`kubergrunt` helps with the certificate management by encapsulating the steps in the `kubergrunt helm grant` command.
This command is used to issue new client TLS certificate key pairs that is trusted by the CA so that they can be used to
authenticate against the deployed Tiller instance. This is done by:

- Downloading the CA certificate key pair.
- Generating new signed certificate key pair for each RBAC entity passed in.
- Storing each new certificate key pair as a `Secret` in the Tiller namespace.

The `Secret` containing the new certificate key pair is shared with the RBAC entity so that they can download it to
configure their client. `kubergrunt` provides the `helm configure` subcommand for your users to use to setup their local
`helm` client with the new certificate key pairs.

Note that `helm grant` should only be run by an administrator of your cluster. This is because only administrators should
have access to the CA certificate key pair, as that enables you to grant anyone access to the deployed Tiller instance.

### Restrict Client and Server Access to the Tiller Namespace

It is tempting to grant your users access to the Tiller namespace. This temptation comes from the challenge of
identifying the minimal set of permissions that the `helm` client needs to communicate with the Tiller instance.

However, granting access to the Tiller namespace has security implications that you should consider. Tiller stores its
own server side certificate key pair in a `Secret` that the Pod needs access to. Otherwise, Tiller will not be able to
serve the TLS certificate or decrypt the packets that come in. If you grant full access to the Tiller namespace, then
you risk exposing these `Secrets` to your users. This is problematic because you can use these certificates to
authenticate against the Tiller instance, circumventing the TLS authentication put in place.

For these reasons, `kubergrunt helm grant` automates the process of granting users RBAC roles that grant them access to
the TLS certificate key pairs and the Tiller instance. The RBAC roles provide the minimal set of privileges necessary to
configure and use the `helm` client against the deployed Tiller instance, while restricting access to the `Secrets` that
Tiller needs to operate.

Additionally, you will want to restrict access by the Tiller instance in the namespace as well. This is because you
don't want to allow your users to deploy a special helm chart that will grant them access to the Tiller namespace. For
example, If you allow the Tiller instance deploy `Pods` and `ServiceAccounts`, then you can deploy a helm chart that
creates enough resources to mount the Tiller `Secrets`.

While `kubergrunt` does not provide any native functionality for the Tiller RBAC roles, you can see [our example in
`terraform-kubernetes-helm`](https://github.com/gruntwork-io/terraform-kubernetes-helm/blob/master/examples/k8s-tiller-minikube/README.md)
for a way to create the Tiller `ServiceAccount` so that it has the minimal set of permissions necessary to operate in
the Tiller namespace.

### Enable TLS Verification in the Client

`kubergrunt helm configure` can be used to configure a local `helm` client to access a deployed Tiller instance. This
involves downloading the respective client TLS certificate key pairs into the `helm` home directory so that it can
authenticate to the Tiller instance.

Once the TLS certificate key pairs are downloaded, you can enable TLS in the `helm` client by using the `--tls` option.
This will look for the certificate key pairs in the `helm` home directory.

Additionally, you will need to pass in the `--tls-verify` option to enable TLS verification of the server. This will use
the CA certificate with the TLS certificate of the server to verify that the client is indeed talking to the intended
Tiller instance.

Since it is cumbersome to always pass in `--tls` and `--tls-verify` to all your `helm` client commands, `kubergrunt helm
configure` will also install an environment file that you can dot source to setup default options everytime you run
`helm`. This environment file will:

- Enable TLS verification to ensure the Tiller instance the client is connecting to is trusted.
- Enable TLS authentication, forwarding the downloaded TLS certificate information to the Tiller instance.
- Set the Tiller namespace so that you don't have to pass it in.
