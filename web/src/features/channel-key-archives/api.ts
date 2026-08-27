import { api, type ApiRequestConfig } from '@/lib/api'

import type { ChannelKeyArchivesResponse } from './types'

const archiveRequestConfig = (
  config: ApiRequestConfig = {}
): ApiRequestConfig => ({
  ...config,
  skipBusinessError: true,
  skipErrorHandler: true,
})

export async function getGlobalChannelKeyArchives(params: {
  page: number
  pageSize: number
  channelId?: number
}): Promise<ChannelKeyArchivesResponse> {
  const response = await api.get(
    '/api/channel/key_archives',
    archiveRequestConfig({
      params: {
        page: params.page,
        page_size: params.pageSize,
        channel_id: params.channelId,
      },
    })
  )
  return response.data
}

export async function restoreGlobalChannelKeyArchive(
  archiveId: number
): Promise<{ success: boolean; message?: string }> {
  const response = await api.post(
    `/api/channel/key_archives/${archiveId}/restore`,
    undefined,
    archiveRequestConfig()
  )
  return response.data
}

export async function getGlobalChannelKeyArchiveSecret(
  archiveId: number,
  proofToken?: string
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const response = await api.post(
    `/api/channel/key_archives/${archiveId}/key`,
    undefined,
    archiveRequestConfig({
      headers: proofToken ? { 'X-Security-Proof': proofToken } : undefined,
    })
  )
  return response.data
}

export async function deleteGlobalChannelKeyArchive(
  archiveId: number
): Promise<{ success: boolean; message?: string }> {
  const response = await api.delete(
    `/api/channel/key_archives/${archiveId}`,
    archiveRequestConfig()
  )
  return response.data
}
