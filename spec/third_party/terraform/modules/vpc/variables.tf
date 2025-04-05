variable "name" {
  type = string
}

variable "vpc_cidr" {
  type = string
}

variable "public_subnet_tags" {
  type    = map(string)
  default = {}
}

variable "private_subnet_tags" {
  type    = map(string)
  default = {}
}

variable "use_nat_instance" {
  type    = bool
  default = true
}

variable "nat_instance_types" {
  type    = list(string)
  default = ["t4g.nano"]
}

variable "extra_tags" {
  type    = map(string)
  default = {}
}
