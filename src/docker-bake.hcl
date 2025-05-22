variable "IMAGE" {
  default = ""
}

variable "TAG" {
  default = ""
}

variable "CACHE_FROM" {
  default = "latest"
}

group "default" {
  targets = ["wireguard-operator"]
}

target "wireguard-operator" {
  platforms  = ["linux/amd64", "linux/arm64"]
  cache-to   = ["type=inline,mod=max"]
  cache-from = ["type=registry,ref=${IMAGE}:${CACHE_FROM}"]
  tags       = ["${IMAGE}:${TAG}"]
}
