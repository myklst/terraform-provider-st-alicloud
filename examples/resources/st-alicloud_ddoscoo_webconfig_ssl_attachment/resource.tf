resource "st-alicloud_ddoscoo_webconfig_ssl_attachment" "bind_ssl" {
  domain  = "test-domain.com"
  cert_id = 12354465
  tls_version = "tls1.2"
  cipher_suites = "improved"
}
