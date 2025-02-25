variable "name" {
  type = string
}

variable "principal" {
  type = map(any)
}

variable "extra_tags" {
  type    = map(string)
  default = {}
}

variable "managed_policies" {
  type    = set(string)
  default = []
}

variable "create_policy" {
  type    = bool
  default = false
}

variable "policy_definition" {
  type = set(object({
    sid       = string,
    actions   = list(string),
    resources = list(string),
  }))
  default = []
}
