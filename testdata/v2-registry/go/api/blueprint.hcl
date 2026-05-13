name        = "go-api"
description = "Go API service with HTTP and optional gRPC"
version     = "2.0.0"
tags        = ["go", "api", "grpc"]

variable "project_name" {
  description = "Name of the project"
  type        = "string"
  required    = true
  validate    = "^[a-z][a-z0-9-]*$"
}

variable "go_module" {
  description = "Go module path"
  type        = "string"
  default     = "github.com/example/v2-api"
}

variable "use_grpc" {
  description = "Include gRPC support?"
  type        = "bool"
  default     = "false"
}

condition {
  when    = !use_grpc
  exclude = ["proto/"]
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
