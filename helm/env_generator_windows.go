// +build windows

package helm

// Powershell Env Var syntax
// See: https://stackoverflow.com/questions/20077820/how-can-i-source-variables-from-a-bat-file-into-a-powershell-script/20078095#20078095
const envFileName = "env.ps1"
const envTemplate = `
$HELM_HOME = '{{ .HelmHome }}'
$TILLER_NAMESPACE = '{{ .TillerNamespace }}'
$HELM_TLS_VERIFY = 'true'
$HELM_TLS_ENABLE = 'true'
`
