resource "st-alicloud_cms_system_event_group_binding" "system_event_group" {
  rule_name          = "test-rule-name"
  contact_group_name = "test-contact-group-name"
  level              = "3"
}
