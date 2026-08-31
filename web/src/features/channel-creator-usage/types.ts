/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export type ChannelCreatorUsageSummary = {
  total_used_quota: number
  total_channels: number
  unknown_creator_channels: number
  creator_count: number
}

export type ChannelCreatorUsageItem = {
  created_by: number
  username: string
  channel_count: number
  enabled_count: number
  disabled_count: number
  used_quota: number
  quota_ratio: number
}

export type ChannelCreatorUsageListResponse = {
  success: boolean
  message?: string
  data?: {
    items: ChannelCreatorUsageItem[]
    total: number
    page: number
    page_size: number
    summary: ChannelCreatorUsageSummary
  }
}

export type ChannelCreatorUsageChannel = {
  id: number
  name: string
  type: number
  type_name: string
  status: number
  group: string
  used_quota: number
  created_time: number
  created_by: number
}

export type ChannelCreatorUsageDetailResponse = {
  success: boolean
  message?: string
  data?: {
    created_by: number
    username: string
    channel_count: number
    used_quota: number
    channels: ChannelCreatorUsageChannel[]
  }
}
