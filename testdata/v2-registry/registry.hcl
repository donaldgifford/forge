name        = "test-blueprints-v2"
description = "Test blueprint registry (HCL2)"

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
  description   = "Go API service with HTTP and optional gRPC (HCL2)"
  version       = "2.0.0"
  tags          = ["go", "api", "grpc"]
  latest_commit = "v2-fixture"
}

blueprint "helm/chart" {
  path          = "helm/chart"
  description   = "Helm chart blueprint demonstrating verbatim {{ }} preservation"
  version       = "2.0.0"
  tags          = ["helm", "k8s"]
  latest_commit = "v2-fixture"
}
