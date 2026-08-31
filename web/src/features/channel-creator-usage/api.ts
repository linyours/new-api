/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { api, type ApiRequestConfig } from '@/lib/api'

import type {
  ChannelCreatorUsageDetailResponse,
  ChannelCreatorUsageListResponse,
} from './types'

const requestConfig = (config: ApiRequestConfig = {}): ApiRequestConfig => ({
  ...config,
  skipBusinessError: true,
  skipErrorHandler: true,
})

export async function getChannelCreatorUsage(params: {
  page: number
  pageSize: number
  keyword?: string
}): Promise<ChannelCreatorUsageListResponse> {
  const response = await api.get(
    '/api/channel/creator_usage',
    requestConfig({
      params: {
        p: params.page,
        page_size: params.pageSize,
        keyword: params.keyword || undefined,
      },
    })
  )
  return response.data
}

export async function getChannelCreatorUsageChannels(
  userId: number
): Promise<ChannelCreatorUsageDetailResponse> {
  const response = await api.get(
    `/api/channel/creator_usage/${userId}/channels`,
    requestConfig()
  )
  return response.data
}

export async function downloadChannelCreatorUsageExport(
  userId: number
): Promise<Blob> {
  const response = await api.get(
    `/api/channel/creator_usage/${userId}/export`,
    requestConfig({
      responseType: 'blob',
    })
  )
  return response.data
}
