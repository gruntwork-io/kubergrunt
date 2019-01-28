// +build linux darwin

package helm

const envFileName = "env"
const envTemplate = `
export HELM_HOME={{ .HelmHome }}
export TILLER_NAMESPACE={{ .TillerNamespace }}
export HELM_TLS_VERIFY=true
export HELM_TLS_ENABLE=true
`
