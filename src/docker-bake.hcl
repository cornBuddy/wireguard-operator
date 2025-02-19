variable "TAG" {
  default = ""
}

variable "IMAGE" {
  default = ""
}

group "default" {
  targets = ["service"]
}

target "service" {
  platforms = ["linux/amd64", "linux/arm64"]
  cache-from = ["type=registry,ref=${IMAGE}:cache"]
  cache-to = [
    "type=registry,ref=${IMAGE}:cache,image-manifest=true,mode=max",
  ]
  tags = [
    "${IMAGE}:${TAG}",
    "${IMAGE}:latest",
  ]
}
