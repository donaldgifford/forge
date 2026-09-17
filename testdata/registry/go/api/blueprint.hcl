name        = "go-api"
description = "Go API service with HTTP and optional gRPC"
version     = "1.0.0"
tags        = ["go", "api", "grpc"]

defaults {
  exclude = [".pre-commit-config.yaml"]
  override_strategy = {
    "renovate.json" = "merge"
  }
}

variable "project_name" {
  description = "Name of the project"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be lowercase letters, digits, or hyphens, starting with a letter."
  }
}

variable "description" {
  description = "Project description"
  type        = "string"
}

variable "go_module" {
  description = "Go module path"
  type        = "string"
  default     = "github.com/example/${project_name}"
}

variable "use_grpc" {
  description = "Include gRPC support?"
  type        = "bool"
  default     = "false"
}

variable "license" {
  description = "License type"
  type        = string
  default     = "Apache-2.0"
  validation {
    condition     = contains(["MIT", "Apache-2.0", "BSD-3-Clause", "none"], var.license)
    error_message = "license must be one of: MIT, Apache-2.0, BSD-3-Clause, none."
  }
}

condition {
  when    = !use_grpc
  exclude = ["proto/", "internal/grpc/"]
}

hooks {
  post_create = ["git init", "go mod tidy"]
}

sync {
  ignore = ["vendor/"]
  managed_file "Makefile" {
    strategy = "merge"
  }
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
