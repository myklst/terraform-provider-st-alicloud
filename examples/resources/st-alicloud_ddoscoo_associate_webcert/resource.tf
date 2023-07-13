resource "st-alicloud_ddoscoo_associate_webcert" "bind_ssl" {
  domain  = "test-domain.com"
  cert_id = 12354465
}
