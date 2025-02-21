variable "IMAGE" {
  default = ""
}

variable "TAG" {
  default = ""
}

group "default" {
  targets = ["wireguard-operator"]
}

target "wireguard-operator" {
  platforms = ["linux/amd64", "linux/arm64"]
  cache-from = ["type=registry,ref=${IMAGE}:cache"]
  cache-to = ["type=registry,ref=${IMAGE}:cache,image-manifest=true,mode=max"]
  tags = ["${IMAGE}:${TAG}"]
}
