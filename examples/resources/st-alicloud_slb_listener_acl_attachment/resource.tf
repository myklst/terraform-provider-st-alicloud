resource "st-alicloud_slb_listener_acl_attachment" "example" {
  load_balancer_id = "lb-1234567890"
  listener_port    = 80
  protocol         = "tcp"
  acl_ids          = ["acl-1234567890", "acl-0987654321"]
}
