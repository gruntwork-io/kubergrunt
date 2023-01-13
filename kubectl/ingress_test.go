package kubectl

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"
)

const ExampleIngressName = "nginx-service-ingress"

func TestGetIngressReturnsErrorForNonExistantIngress(t *testing.T) {
	t.Parallel()

	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	kubectlOptions := &KubectlOptions{ConfigPath: kubeConfigPath}
	_, err = GetIngress(kubectlOptions, "kube-system", "i-dont-exist")
	require.Error(t, err)
}

func TestGetIngressEReturnsCorrectIngressInCorrectNamespace(t *testing.T) {
	t.Parallel()

	uniqueID := strings.ToLower(random.UniqueId())
	ttKubectlOptions := k8s.NewKubectlOptions("", "", uniqueID)
	configData := fmt.Sprintf(
		exampleIngressDeploymentYAMLTemplate,
		uniqueID, uniqueID, uniqueID, uniqueID, uniqueID,
	)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)
	k8s.KubectlApplyFromString(t, ttKubectlOptions, configData)

	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	kubectlOptions := &KubectlOptions{ConfigPath: kubeConfigPath}

	ingress, err := GetIngress(kubectlOptions, uniqueID, "nginx-service-ingress")
	require.NoError(t, err)
	require.Equal(t, ingress.Name, "nginx-service-ingress")
	require.Equal(t, ingress.Namespace, uniqueID)
}

func TestWaitUntilIngressAvailableReturnsSuccessfully(t *testing.T) {
	t.Parallel()

	uniqueID := strings.ToLower(random.UniqueId())
	ttKubectlOptions := k8s.NewKubectlOptions("", "", uniqueID)
	configData := fmt.Sprintf(
		exampleIngressDeploymentYAMLTemplate,
		uniqueID, uniqueID, uniqueID, uniqueID, uniqueID,
	)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)
	k8s.KubectlApplyFromString(t, ttKubectlOptions, configData)

	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	kubectlOptions := &KubectlOptions{ConfigPath: kubeConfigPath}

	err = WaitUntilIngressEndpointProvisioned(kubectlOptions, uniqueID, ExampleIngressName, 60, 5*time.Second)
	require.NoError(t, err)
}

const exampleIngressDeploymentYAMLTemplate = `---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: %s
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 1
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.15.7
        ports:
        - containerPort: 80
---
kind: Service
apiVersion: v1
metadata:
  name: nginx-service
  namespace: %s
spec:
  selector:
    app: nginx
  ports:
  - protocol: TCP
    targetPort: 80
    port: 80
  type: NodePort
---
kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: nginx-service-ingress
  namespace: %s
spec:
  rules:
  - http:
      paths:
      - path: /app%s
        backend:
          serviceName: nginx-service
          servicePort: 80
`
