# Instance-level burst (no node_id → defaults to "All")
resource "st-alicloud_kvstore_additional_bandwidth" "burst" {
  instance_id     = "r-xxxxx"
  bandwidth_burst = true
}

# Per-shard additional bandwidth
resource "st-alicloud_kvstore_additional_bandwidth" "shard_0" {
  instance_id = "r-xxxxx"
  node_id     = "r-xxxxx-db-0"
  bandwidth   = 20
}
