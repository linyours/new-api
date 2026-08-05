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
  ChannelLayeredBatchData,
  LayeredDimension,
  LayeredMetricsData,
  PerformanceMetricsData,
  PerfSummaryAllData,
} from './types'

export async function getPerfMetricsSummary(
  hours = 24
): Promise<PerfSummaryAllData> {
  const res = await api.get<PerfSummaryAllData>('/api/perf-metrics/summary', {
    params: { hours },
  })
  return res.data
}

export async function getPerfMetrics(
  modelName: string,
  hours = 24
): Promise<PerformanceMetricsData> {
  const res = await api.get<PerformanceMetricsData>('/api/perf-metrics', {
    params: {
      model: modelName,
      hours,
    },
  })
  return res.data
}

export async function getPerfMetricsLayered(params: {
  dimension: LayeredDimension
  channel_type?: number
  channel_id?: number
  model?: string
}): Promise<LayeredMetricsData> {
  const res = await api.get<LayeredMetricsData>('/api/perf-metrics/layered', {
    params: {
      dimension: params.dimension,
      channel_type: params.channel_type,
      channel_id: params.channel_id,
      model: params.model,
    },
  })
  return res.data
}

export async function getPerfMetricsLayeredChannels(params: {
  ids: number[]
  windowSeconds?: number
}): Promise<ChannelLayeredBatchData> {
  const res = await api.get<ChannelLayeredBatchData>(
    '/api/perf-metrics/layered/channels',
    {
      params: {
        ids: params.ids.join(','),
        window: params.windowSeconds ?? 3600,
      },
    }
  )
  return res.data
}
