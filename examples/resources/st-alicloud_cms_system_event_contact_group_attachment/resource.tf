resource "st-alicloud_cms_system_event_contact_group_attachment" "contact_group_attachment" {
  rule_name          = "test-rule-name"
  contact_group_name = "test-contact-group-name"
  level              = "3"
}
