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

variable "nat_instance_distribution" {
  type = object({
    on_demand_base_capacity                  = optional(number, 1),
    on_demand_percentage_above_base_capacity = optional(number, 100),
  })
  default = {}
}

variable "extra_tags" {
  type    = map(string)
  default = {}
}
