name        = "test-blueprints-hcl"
description = "Test blueprint registry (HCL config files)"

maintainer {
  name  = "Test Team"
  email = "test@example.com"
}

defaults {
  sync_strategy = "overwrite"
  managed       = true
}

blueprint "go/api" {
  path          = "go/api"
  description   = "Go API service with HTTP and optional gRPC (HCL config)"
  version       = "0.4.0"
  tags          = ["go", "api", "grpc"]
  latest_commit = "hcl-fixture"
}

blueprint "helm/chart" {
  path          = "helm/chart"
  description   = "Helm chart blueprint demonstrating verbatim {{ }} preservation"
  version       = "0.4.0"
  tags          = ["helm", "k8s"]
  latest_commit = "hcl-fixture"
}
