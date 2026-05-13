name        = "helm-chart"
description = "Helm chart blueprint with verbatim {{ .Values.x }} preservation"
version     = "0.4.0"
tags        = ["helm", "k8s"]

variable "project_name" {
  description = "Name of the chart"
  type        = "string"
  required    = true
  validate    = "^[a-z][a-z0-9-]*$"
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
