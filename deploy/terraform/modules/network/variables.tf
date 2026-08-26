variable "project_id" {
  type        = string
  description = "GCP project to create the network in."
}

variable "region" {
  type        = string
  description = "Region for the subnet and VPC connector."
}

variable "network_name" {
  type        = string
  description = "Base name for the VPC and its child resources."
  default     = "marginal-vpc"
}

variable "subnet_cidr" {
  type        = string
  description = "CIDR for the single regional subnet."
  default     = "10.10.0.0/24"
}

variable "connector_cidr" {
  type        = string
  description = "CIDR for the Serverless VPC Access connector. Must be a /28 and must not overlap subnet_cidr."
  default     = "10.10.1.0/28"
}
