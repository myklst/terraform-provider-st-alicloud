data "st-alicloud_cs_user_kubeconfig" "def" {
  cluster_id = "c-123"

  client_config {
    region     = "cn-hongkong"
    access_key = "<access-key>"
    secret_key = "<secret-key>"
  }
}
