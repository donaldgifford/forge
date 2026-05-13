name        = "test-blueprints"
description = "Test blueprint registry"

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
  description   = "Go API service with HTTP and optional gRPC"
  version       = "1.0.0"
  tags          = ["go", "api", "grpc"]
  latest_commit = "abc123def456"
}

blueprint "go/cli" {
  path          = "go/cli"
  description   = "Go CLI application with Cobra"
  version       = "1.0.0"
  tags          = ["go", "cli", "cobra"]
  latest_commit = "abc123def456"
}
