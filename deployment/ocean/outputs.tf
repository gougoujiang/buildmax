output "project_name" {
  value = data.digitalocean_project.persistent.name
}

output "region" {
  value = var.region
}

output "kubernetes_cluster_name" {
  value = digitalocean_kubernetes_cluster.beta.name
}

output "kubernetes_endpoint" {
  value = digitalocean_kubernetes_cluster.beta.endpoint
}

output "kubeconfig" {
  value     = digitalocean_kubernetes_cluster.beta.kube_config[0].raw_config
  sensitive = true
}

output "database_private_host" {
  value = digitalocean_database_cluster.beta.private_host
}

output "database_port" {
  value = digitalocean_database_cluster.beta.port
}

output "database_user" {
  value     = digitalocean_database_cluster.beta.user
  sensitive = true
}

output "database_password" {
  value     = digitalocean_database_cluster.beta.password
  sensitive = true
}

output "database_ca" {
  value     = data.digitalocean_database_ca.beta.certificate
  sensitive = true
}

output "database_name" {
  value = digitalocean_database_db.buildmax.name
}

output "spaces_bucket_name" {
  value = data.digitalocean_spaces_bucket.persistent.name
}

output "spaces_endpoint" {
  value = "https://${var.region}.digitaloceanspaces.com"
}
