name        = "helm-chart"
description = "Helm chart blueprint with verbatim {{ .Values.x }} preservation"
version     = "2.0.0"
tags        = ["helm", "k8s"]

variable "project_name" {
  description = "Name of the chart"
  type        = string
  required    = true
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.project_name))
    error_message = "project_name must be lowercase letters, digits, or hyphens, starting with a letter."
  }
}

variable "app_image" {
  description = "Default container image"
  type        = "string"
  default     = "nginx"
}

rename {
  entry {
    from = "${project_name}/"
    to   = "."
  }
}
