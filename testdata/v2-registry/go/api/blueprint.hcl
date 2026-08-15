name        = "go-api"
description = "Go API service with HTTP and optional gRPC"
version     = "2.0.0"
tags        = ["go", "api", "grpc"]

variable "project_name" {
  description = "Name of the project"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be lowercase letters, digits, or hyphens, starting with a letter."
  }
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
