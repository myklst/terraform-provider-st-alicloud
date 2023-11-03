resource "st-alicloud_ess_clb_default_server_group_attachment" "example" {
  scaling_group_id  = "asg-xxxxxxxxxxxxxxxxxxxx"
  load_balancer_ids = ["lb-xxxxxxxxxxxxxxxxxxxxx"]
}
