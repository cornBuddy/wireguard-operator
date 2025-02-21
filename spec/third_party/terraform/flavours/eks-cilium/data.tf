data "aws_iam_policy" "this" {
  for_each = toset(["AmazonEBSCSIDriverPolicy", "AmazonSSMManagedInstanceCore"])

  name = each.value
}
