$ErrorActionPreference = "Stop"

$chartVersion = "28.15.0"
$releaseName = "auto-prometheus"
$namespace = "observability"
kubectl apply -f .\namespace.yaml

helm upgrade --install $releaseName prometheus-community/prometheus `
  --version $chartVersion `
  --namespace $namespace `
  --create-namespace `
  -f .\values.yaml


