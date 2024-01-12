resource "st-alicloud_service_mesh_user_permission" "default" {
  sub_account_user_id = "201122334455667789"

  permissions {
    role_name       = "istio-admin"
    service_mesh_id = "cxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    role_type       = "custom"
    is_custom       = true
    is_ram_role     = false
  }
}
