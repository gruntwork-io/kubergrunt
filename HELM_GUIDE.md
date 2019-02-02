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

- Deploy Tiller into its own `Namespace`, separately from the namespace where it will allow helm charts to deploy into.
- Enable RBAC and restrict what Tiller can do by creating its own `ServiceAccount`.
- Enable TLS verification in the server so that only authorized clients can access it.
- Enable TLS verification in the client so that it will only access servers that it trusts.
- Ensure Tiller stores its metadata in `Secrets`.

`kubergrunt` includes various commands to help enable those best practices.

### Deploy

`kubergrunt helm deploy` will deploy Tiller with all the security options turned on. This includes:

- Forcing the operator to pass in the Tiller namespace (the `Namespace` where Tiller will be deployed into), the
  resource namespace (the `Namespace` where Tiller will be allowed to deploy resources into), and a `ServiceAccount`
  (the account that Tiller will use to authenticate with the Kubernetes API). By doing so, `kubergrunt` forces the
  operator to be conscious of what they decide to allow Tiller to do in the cluster.
- Enabling TLS verification on the server side.
- Configuring Tiller to store its metadata into `Secrets` as opposed to `ConfigMaps`.

As a part of this, the `kubergrunt` command will generate two new TLS certificate key pairs:

- a key pair that can be used as the CA for verifying the server and the clients.
- a key pair that can be used to identify the server.

The private key for the generated CA key pair will be stored in the Kubernetes cluster as a `Secret` in the
`kube-system` namespace. Doing so allows future operators to generate new certificate key pairs to grant additional
clients access to the deployed Tiller instance, while protecting access to the CA key pair using the native RBAC system
of the cluster. By placing the CA key pair in the `kube-system` namespace, it makes it difficult to accidentally grant
access to it outside your group of trusted operators (the cluster admins).

The certificate key pair that Tiller will use will be placed into a `Secret` in the Tiller namespace so that the Tiller
pod may access it. This key pair can be used to authenticate against the Tiller server. As such, it is important to
lock down access to the Tiller namespace. However, in order for the `helm` client to access the Tiller instance, you may
need to grant certain permissions to the user to be able to create a connection to the Tiller pod. `kubergrunt` handles
this in the `grant` command.

### Grant

`kubergrunt helm grant` is used to issue new client TLS certificate key pairs that is trusted by the CA so that they can
be used to authenticate against the deployed Tiller instance. This is done by:

- Downloading the CA certificate key pair.
- Generating new signed certificate key pair for each RBAC entity passed in.
- Storing each new certificate key pair as a `Secret` in the Tiller namespace.

We recommend that your users should not have access to the Tiller namespace. However, if they don't have access, how do
they get the client certificate key pairs? As part of the granting process, `kubergrunt` will also bind a minimal set of
RBAC permissions in the Tiller namespace. These are:

- Read access to the `Secret` containing the client certificate. The user should only be granted access to the one
  pertaining to that user.
- Read and List access to `Pods` in the Tiller namespace. This is necessary by the `helm` client to find the Tiller pod.
- Create access for a port forward to the Tiller pod. This is necessary by the `helm` client to open a tunnel to the
  Tiller pod.

By doing so, we are able to allow the `helm` client to connect and make requests to Tiller, while restricting the user
from getting access to other client certificates, the Tiller server certificates, or the Tiller metadata.

### Configure

`kubergrunt helm configure` is used to configure a local `helm` client to access a deployed Tiller instance. This
involves downloading the respective client TLS certificate key pairs so that it can authenticate to the Tiller instance.

In addition to setting up the TLS authentication, `kubergrunt` will also install an environment file that you can dot
source to setup default options everytime you run `helm`. This environment file will:

- Enable TLS verification to ensure the Tiller instance the client is connecting to is trusted.
- Enable TLS authentication, forwarding the downloaded TLS certificate information to the Tiller instance.
- Set the Tiller namespace so that you don't have to pass it in.
