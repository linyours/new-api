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
import { useQuery } from '@tanstack/react-query'
import { Layers } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'
import { getPerfMetricsLayered } from '@/features/performance-metrics/api'
import {
  formatUptimePct,
  getSuccessRateDotClass,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { LayeredDimension } from '@/features/performance-metrics/types'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

export function LayeredSuccessRatePanel() {
  const { t } = useTranslation()
  const [dimension, setDimension] = useState<LayeredDimension>('all')
  const [channelType, setChannelType] = useState<string>('')
  const [channelIdInput, setChannelIdInput] = useState('')
  const [modelInput, setModelInput] = useState('')

  const channelId = useMemo(() => {
    const parsed = Number.parseInt(channelIdInput, 10)
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
  }, [channelIdInput])

  const channelTypeValue = useMemo(() => {
    const parsed = Number.parseInt(channelType, 10)
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
  }, [channelType])

  const modelName = useMemo(() => modelInput.trim(), [modelInput])

  const queryEnabled =
    dimension === 'all' ||
    (dimension === 'channel_type' && channelTypeValue > 0) ||
    (dimension === 'channel' && channelId > 0) ||
    (dimension === 'model' && modelName !== '')

  const metricsQuery = useQuery({
    queryKey: [
      'perf-metrics-layered',
      dimension,
      channelTypeValue,
      channelId,
      modelName,
    ],
    queryFn: async () => {
      const res = await getPerfMetricsLayered({
        dimension,
        channel_type:
          dimension === 'channel_type' ? channelTypeValue : undefined,
        channel_id: dimension === 'channel' ? channelId : undefined,
        model: dimension === 'model' ? modelName : undefined,
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to load layered success rate'))
      }
      return res.data
    },
    enabled: queryEnabled,
    staleTime: 15 * 1000,
    refetchInterval: 15 * 1000,
    retry: false,
  })

  const windows = metricsQuery.data?.windows ?? []
  const asOf = metricsQuery.data?.as_of
  const loading = metricsQuery.isLoading || metricsQuery.isFetching

  const filterHint = (() => {
    switch (dimension) {
      case 'channel_type':
        return t('Select a provider type to view layered success rates.')
      case 'channel':
        return t('Enter a channel ID to view layered success rates.')
      case 'model':
        return t('Enter a model name to view layered success rates.')
      default:
        return ''
    }
  })()

  return (
    <section className='bg-card h-full overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center gap-2 border-b px-4 py-3 sm:px-5'>
        <IconBadge tone='warning' size='sm'>
          <Layers />
        </IconBadge>
        <h3 className='text-sm font-semibold'>{t('Layered success rate')}</h3>
        <span className='text-muted-foreground ml-auto text-xs'>
          {asOf
            ? t('Data as of {{time}}', {
                time: formatTimestampToDate(asOf),
              })
            : t('Waiting for filter selection')}
        </span>
      </div>

      <div className='space-y-3 p-4 sm:p-5'>
        <Tabs
          value={dimension}
          onValueChange={(value) => {
            if (
              value === 'all' ||
              value === 'channel_type' ||
              value === 'channel' ||
              value === 'model'
            ) {
              setDimension(value)
            }
          }}
        >
          <TabsList className='h-auto flex-wrap justify-start'>
            <TabsTrigger value='all'>{t('All traffic')}</TabsTrigger>
            <TabsTrigger value='channel_type'>{t('Provider type')}</TabsTrigger>
            <TabsTrigger value='channel'>{t('Channel')}</TabsTrigger>
            <TabsTrigger value='model'>{t('Model')}</TabsTrigger>
          </TabsList>
        </Tabs>

        {dimension === 'channel_type' && (
          <Select
            value={channelType}
            onValueChange={(value) => setChannelType(value ?? '')}
          >
            <SelectTrigger className='w-full sm:w-72'>
              <SelectValue placeholder={t('Select provider type')} />
            </SelectTrigger>
            <SelectContent>
              {CHANNEL_TYPE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={String(option.value)}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {dimension === 'channel' && (
          <Input
            type='number'
            min={1}
            inputMode='numeric'
            className='w-full sm:w-72'
            placeholder={t('Enter channel ID')}
            value={channelIdInput}
            onChange={(event) => setChannelIdInput(event.target.value)}
          />
        )}

        {dimension === 'model' && (
          <Input
            className='w-full sm:w-72'
            placeholder={t('Enter model name')}
            value={modelInput}
            onChange={(event) => setModelInput(event.target.value)}
          />
        )}

        {!queryEnabled ? (
          <p className='text-muted-foreground text-xs'>{filterHint}</p>
        ) : loading && windows.length === 0 ? (
          <div className='space-y-2'>
            {['1min', '5min', '15min', '30min', '1h'].map((key) => (
              <Skeleton key={key} className='h-8 w-full rounded-lg' />
            ))}
          </div>
        ) : metricsQuery.isError ? (
          <p className='text-destructive text-xs'>
            {metricsQuery.error instanceof Error
              ? metricsQuery.error.message
              : t('Failed to load layered success rate')}
          </p>
        ) : (
          <ul className='space-y-1.5'>
            {windows.map((window) => (
              <li
                key={window.label}
                className='bg-muted/40 flex items-center gap-3 rounded-xl px-3 py-2'
              >
                <span className='text-muted-foreground w-12 shrink-0 font-mono text-xs font-medium'>
                  {window.label}
                </span>
                <span
                  className={cn(
                    'size-2 shrink-0 rounded-full',
                    getSuccessRateDotClass(window.success_rate)
                  )}
                  aria-hidden='true'
                />
                <span
                  className={cn(
                    'min-w-16 font-mono text-sm font-semibold tabular-nums',
                    getSuccessRateTextClass(window.success_rate)
                  )}
                >
                  {formatUptimePct(window.success_rate)}
                </span>
                <span className='text-muted-foreground ml-auto font-mono text-xs tabular-nums'>
                  {t('{{count}} requests', {
                    count: window.request_count,
                  })}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
