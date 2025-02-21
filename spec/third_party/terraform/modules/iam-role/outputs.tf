output "arn" {
  description = "ARN of role created"
  value       = aws_iam_role.this.arn
}

output "name" {
  description = "Name of role created"
  value       = aws_iam_role.this.name
}

output "policy_arn" {
  description = "ARN of IAM policy created"
  value       = var.create_policy ? aws_iam_policy.this[0].arn : null
}
