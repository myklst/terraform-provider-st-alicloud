resource "st-alicloud_slb_listener_whitelist_acl_attachment" "example" {
  listener_id = "lb-1234567890:tcp:80"
  acl_ids     = ["acl-1234567890", "acl-0987654321"]
}
