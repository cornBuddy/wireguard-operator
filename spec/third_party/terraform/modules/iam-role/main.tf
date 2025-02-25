resource "aws_iam_role" "this" {
  name                  = var.name
  force_detach_policies = true
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Action    = "sts:AssumeRole",
        Effect    = "Allow",
        Principal = var.principal,
      },
    ],
  })
  tags = var.extra_tags
}

data "aws_iam_policy" "managed" {
  for_each = var.managed_policies

  name = each.value
}

resource "aws_iam_role_policy_attachment" "managed" {
  for_each = toset(values(data.aws_iam_policy.managed)[*].arn)

  role       = aws_iam_role.this.name
  policy_arn = each.value
}

data "aws_iam_policy_document" "this" {
  count = var.create_policy ? 1 : 0

  dynamic "statement" {
    for_each = var.policy_definition
    content {
      sid       = statement.value["sid"]
      actions   = statement.value["actions"]
      resources = statement.value["resources"]
    }
  }
}

resource "aws_iam_policy" "this" {
  count = var.create_policy ? 1 : 0

  name   = var.name
  path   = "/"
  policy = data.aws_iam_policy_document.this[0].json
  tags   = var.extra_tags
}

resource "aws_iam_role_policy_attachment" "this" {
  count = var.create_policy ? 1 : 0

  role       = aws_iam_role.this.name
  policy_arn = aws_iam_policy.this[0].arn
}
