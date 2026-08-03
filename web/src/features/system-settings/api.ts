/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import type {
  ChannelDisableMonitorConfig,
  ChannelDisableMonitorConfigResponse,
  ConfirmPaymentComplianceResponse,
  DeleteGroupChannelFallbackRequest,
  FetchUpstreamRatiosRequest,
  GroupChannelFallbackListResponse,
  LogCleanupTask,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  TTFTMonitorConfig,
  TTFTMonitorConfigResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpsertGroupChannelFallbackRequest,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse<LogCleanupTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}

export async function getTTFTMonitorConfig() {
  const res = await api.get<TTFTMonitorConfigResponse>(
    '/api/monitor/ttft/config'
  )
  return res.data
}

export async function updateTTFTMonitorConfig(request: TTFTMonitorConfig) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/monitor/ttft/config',
    request
  )
  return res.data
}

export async function getChannelDisableMonitorConfig() {
  const res = await api.get<ChannelDisableMonitorConfigResponse>(
    '/api/monitor/channel_disable/config'
  )
  return res.data
}

export async function updateChannelDisableMonitorConfig(
  request: ChannelDisableMonitorConfig
) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/monitor/channel_disable/config',
    request
  )
  return res.data
}

export async function getGroupChannelFallbacks() {
  const res = await api.get<GroupChannelFallbackListResponse>(
    '/api/channel/group_fallback'
  )
  return res.data
}

export async function upsertGroupChannelFallback(
  request: UpsertGroupChannelFallbackRequest
) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/channel/group_fallback',
    request
  )
  return res.data
}

export async function deleteGroupChannelFallback(
  request: DeleteGroupChannelFallbackRequest
) {
  const res = await api.delete<UpdateOptionResponse>(
    '/api/channel/group_fallback',
    {
      data: request,
    }
  )
  return res.data
}
