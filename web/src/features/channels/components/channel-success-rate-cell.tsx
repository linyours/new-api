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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import { getPerfMetricsLayered } from '@/features/performance-metrics/api'
import {
  formatUptimePct,
  getSuccessRateDotClass,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { ChannelLayeredMetric } from '@/features/performance-metrics/types'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

type ChannelSuccessRateCellProps = {
  channelId: number
  channelName?: string
  metric?: ChannelLayeredMetric
  loading?: boolean
}

export function ChannelSuccessRateCell(props: ChannelSuccessRateCellProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  const layeredQuery = useQuery({
    queryKey: ['perf-metrics-layered', 'channel', props.channelId],
    queryFn: async () => {
      const res = await getPerfMetricsLayered({
        dimension: 'channel',
        channel_id: props.channelId,
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to load layered success rate'))
      }
      return res.data
    },
    enabled: open && props.channelId > 0,
    staleTime: 15 * 1000,
    retry: false,
  })

  const windows = layeredQuery.data?.windows ?? []
  const asOf = layeredQuery.data?.as_of
  const hasMetric =
    props.metric != null &&
    Number.isFinite(props.metric.request_count) &&
    props.metric.request_count > 0

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        className={cn(
          'inline-flex items-center gap-1.5 rounded-md px-1 py-0.5 text-left',
          'hover:bg-muted/60 focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none',
          'cursor-pointer'
        )}
        title={t('Click to view layered success rate')}
      >
        {props.loading && !hasMetric ? (
          <span className='text-muted-foreground text-xs'>…</span>
        ) : hasMetric ? (
          <>
            <span
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                getSuccessRateDotClass(props.metric!.success_rate)
              )}
              aria-hidden='true'
            />
            <span
              className={cn(
                'font-mono text-xs font-semibold tabular-nums underline-offset-2 hover:underline',
                getSuccessRateTextClass(props.metric!.success_rate)
              )}
            >
              {formatUptimePct(props.metric!.success_rate)}
            </span>
          </>
        ) : (
          <span className='text-muted-foreground text-xs underline-offset-2 hover:underline'>
            —
          </span>
        )}
      </PopoverTrigger>
      <PopoverContent align='start' side='bottom' className='w-72 p-3'>
        <PopoverHeader className='mb-2 gap-0.5'>
          <PopoverTitle className='text-sm'>
            {t('Layered success rate')}
          </PopoverTitle>
          <PopoverDescription className='text-xs'>
            {props.channelName
              ? `#${props.channelId} · ${props.channelName}`
              : `#${props.channelId}`}
            {asOf
              ? ` · ${t('Data as of {{time}}', {
                  time: formatTimestampToDate(asOf),
                })}`
              : null}
          </PopoverDescription>
        </PopoverHeader>

        {layeredQuery.isLoading ? (
          <div className='space-y-1.5'>
            {['1min', '5min', '15min', '30min', '1h'].map((key) => (
              <Skeleton key={key} className='h-8 w-full rounded-lg' />
            ))}
          </div>
        ) : layeredQuery.isError ? (
          <p className='text-destructive text-xs'>
            {layeredQuery.error instanceof Error
              ? layeredQuery.error.message
              : t('Failed to load layered success rate')}
          </p>
        ) : (
          <ul className='space-y-1'>
            {windows.map((window) => (
              <li
                key={window.label}
                className='bg-muted/40 flex items-center gap-2 rounded-lg px-2.5 py-1.5'
              >
                <span className='text-muted-foreground w-10 shrink-0 font-mono text-[11px] font-medium'>
                  {window.label}
                </span>
                <span
                  className={cn(
                    'size-1.5 shrink-0 rounded-full',
                    getSuccessRateDotClass(window.success_rate)
                  )}
                  aria-hidden='true'
                />
                <span
                  className={cn(
                    'min-w-14 font-mono text-xs font-semibold tabular-nums',
                    getSuccessRateTextClass(window.success_rate)
                  )}
                >
                  {window.request_count > 0
                    ? formatUptimePct(window.success_rate)
                    : '—'}
                </span>
                <span className='text-muted-foreground ml-auto font-mono text-[11px] tabular-nums'>
                  {t('{{count}} requests', {
                    count: window.request_count,
                  })}
                </span>
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  )
}
