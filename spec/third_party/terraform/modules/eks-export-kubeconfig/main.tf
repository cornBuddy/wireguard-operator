resource "null_resource" "this" {
  provisioner "local-exec" {
    command = "aws eks update-kubeconfig --name ${var.cluster_name}"
  }
}
