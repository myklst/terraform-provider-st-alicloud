---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "st-alicloud_ram_user_group_attachment Resource - st-alicloud"
subcategory: ""
description: |-
  Provides a Alicloud RAM User Group Attachment resource.
---

# st-alicloud_ram_user_group_attachment (Resource)

Provides a Alicloud RAM User Group Attachment resource.

## Example Usage

```terraform
resource "st-alicloud_ram_user_group_attachment" "ram_group" {
  group_name = "test-group"
  user_name  = "test-user"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `group_name` (String) The group name.
- `user_name` (String) The username of the RAM group member.
