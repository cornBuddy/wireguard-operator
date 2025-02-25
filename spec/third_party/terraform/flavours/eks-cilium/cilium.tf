resource "helm_release" "cilium" {
  name              = "cilium"
  chart             = "cilium"
  repository        = "https://helm.cilium.io/"
  version           = "1.17.1"
  namespace         = "kube-system"
  wait              = true
  dependency_update = true
  values = [
    file("${path.module}/files/cilium-base-values.yml"),
  ]
}

resource "null_resource" "cilium" {
  triggers = {
    always = timestamp()
  }

  provisioner "local-exec" {
    command = "${path.module}/files/delete-vpc-cni.sh"
  }
}

