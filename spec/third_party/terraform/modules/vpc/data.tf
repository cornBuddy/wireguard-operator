data "aws_default_tags" "this" {}

data "aws_ami" "nat_instance" {
  most_recent = true
  owners      = ["self"]

  filter {
    name   = "name"
    values = ["nat-instance-*"]
  }
  filter {
    name   = "architecture"
    values = ["arm64"]
  }
}

data "aws_availability_zones" "this" {
  state = "available"
}
