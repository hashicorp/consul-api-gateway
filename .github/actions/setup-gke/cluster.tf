variable "project" {
  type = string
  nullable = false
}

variable "cluster_prefix" {
  type = string
  nullable = false
}

variable "region" {
  type = string
  nullable = false
  default = "us-west1"
}

variable "cluster_count" {
  default = 1
}

provider "google-beta" {
  project = var.project
  version = "~> 3.49.0"
}

resource "random_id" "suffix" {
  count       = var.cluster_count
  byte_length = 4
}

data "google_container_engine_versions" "main" {
  location       = var.region
  version_prefix = "1.23."
}

resource "google_container_cluster" "cluster" {
  provider = "google-beta"

  count = var.cluster_count

  name               = "${var.cluster_prefix}-${random_id.suffix[count.index].dec}"
  project            = var.project
  initial_node_count = 3
  location           = var.region
  min_master_version = data.google_container_engine_versions.main.latest_master_version
  node_version       = data.google_container_engine_versions.main.latest_master_version
  node_config {
    machine_type = "e2-standard-4"
  }
  pod_security_policy_config {
    enabled = true
  }

}

resource "google_compute_firewall" "firewall-rules" {
  project     = var.project
  name        = "${var.cluster_prefix}-${random_id.suffix[count.index].dec}"
  network     = "default"
  description = "Creates firewall rule allowing traffic from nodes and pods of the ${random_id.suffix[count.index == 0 ? 1 : 0].dec} Kubernetes cluster."

  count = var.cluster_count > 1 ? var.cluster_count : 0

  allow {
    protocol = "all"
  }

  source_ranges = [google_container_cluster.cluster[count.index == 0 ? 1 : 0].cluster_ipv4_cidr]
  source_tags   = ["consul-k8s-${random_id.suffix[count.index == 0 ? 1 : 0].dec}"]
  target_tags   = ["consul-k8s-${random_id.suffix[count.index].dec}"]
}