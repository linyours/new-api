export type ArchivedChannelKey = {
  archive_id: number
  original_key_id: number
  channel_id: number
  channel_name: string
  index: number
  key_preview: string
  rpm_limit?: number | null
  model_rpm_limits: Record<string, number>
  quota_limit?: number | null
  effective_quota: number
  quota_used: number
  lifetime_quota: number
  reason: string
  exhausted_time: number
  archived_time: number
}

export type ChannelKeyArchivesResponse = {
  success: boolean
  message?: string
  data?: {
    keys: ArchivedChannelKey[]
    total: number
    page: number
    page_size: number
    total_pages: number
  }
}
