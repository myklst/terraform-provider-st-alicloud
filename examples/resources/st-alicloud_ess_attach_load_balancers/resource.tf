resource "st-alicloud_ess_attach_load_balancers" "example" {
  scaling_group_id  = "asg-xxxxxxxxxxxxxxxxxxxx"
  load_balancer_ids = ["lb-xxxxxxxxxxxxxxxxxxxxx"]
}
