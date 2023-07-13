resource "st-alicloud_ddoscoo_webconfig_ssl_attachment" "bind_ssl" {
  domain  = "test-domain.com"
  cert_id = 12354465
}
