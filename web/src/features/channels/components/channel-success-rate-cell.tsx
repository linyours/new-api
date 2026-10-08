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
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, CircleAlert, CircleCheck, Layers } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getChannelRecentErrors } from '../api'
import {
  canInspectChannelRecentErrors,
  channelHealthLayerLabel,
  channelsQueryKeys,
  formatRelativeTime,
  isTagAggregateRow,
  recentErrorAccentClass,
  recentErrorStatusBadgeVariant,
  successRateBadgeVariant,
  successRateBarClass,
  successRateHeaderTint,
} from '../lib'
import type {
  Channel,
  ChannelHealth,
  ChannelHealthLayer,
  ChannelRecentErrorItem,
} from '../types'

export function ChannelSuccessRateCell({ channel }: { channel: Channel }) {
  const health = channel.health
  if (!health) {
    return <span className='text-muted-foreground text-xs'>--</span>
  }

  const variant = health.sample_ready
    ? successRateBadgeVariant(health.success_rate, health.alert_below)
    : 'neutral'
  const percentLabel = health.sample_ready
    ? `${Math.round(health.success_rate * 100)}%`
    : '--'

  return (
    <ChannelSuccessRatePopover
      channel={channel}
      health={health}
      percentLabel={percentLabel}
      variant={variant}
    />
  )
}

function ChannelSuccessRatePopover({
  channel,
  health,
  percentLabel,
  variant,
}: {
  channel: Channel
  health: ChannelHealth
  percentLabel: string
  variant: ReturnType<typeof successRateBadgeVariant>
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const canInspect = canInspectChannelRecentErrors(channel)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const isTagRow = isTagAggregateRow(channel)

  const errorsQuery = useQuery({
    queryKey: channelsQueryKeys.recentErrors(channel.id),
    queryFn: async () => {
      const res = await getChannelRecentErrors(channel.id)
      if (!res.success) {
        throw new Error(res.message || 'Failed to load recent errors')
      }
      return res.data ?? { items: [], window_seconds: 300 }
    },
    enabled: open && canInspect,
    staleTime: 20_000,
    retry: 1,
  })

  const successPct =
    health.total > 0 ? Math.min(100, (health.success / health.total) * 100) : 0

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type='button'
            className='focus-visible:ring-ring/50 -ml-1.5 cursor-pointer rounded-4xl focus-visible:ring-[3px] focus-visible:outline-none'
            aria-label={`${percentLabel} ${t('Success rate')}. ${t('Click to view recent errors')}`}
          />
        }
      >
        {health.sample_ready ? (
          <StatusBadge
            label={percentLabel}
            variant={variant}
            size='sm'
            copyable={false}
            className='cursor-pointer'
          />
        ) : (
          <span className='text-muted-foreground text-xs'>--</span>
        )}
      </PopoverTrigger>
      <PopoverContent
        align='start'
        side='bottom'
        sideOffset={8}
        collisionPadding={12}
        className='w-[min(24rem,calc(100vw-1.5rem))] gap-0 overflow-hidden p-0'
      >
        <div
          className={cn(
            'bg-linear-to-br px-3.5 py-3',
            successRateHeaderTint(variant)
          )}
        >
          <PopoverHeader className='gap-1'>
            <div className='flex items-baseline gap-2'>
              <PopoverTitle className='text-2xl font-semibold tracking-tight tabular-nums'>
                {percentLabel}
              </PopoverTitle>
              <span className='text-muted-foreground text-xs'>
                {t('Success rate')}
              </span>
            </div>
            <PopoverDescription className='text-xs'>
              {t('{{success}} / {{total}} in 5 minutes', {
                success: health.success,
                total: health.total,
              })}
              {health.bad > 0
                ? ` · ${t('Counted errors: {{count}}', { count: health.bad })}`
                : null}
            </PopoverDescription>
          </PopoverHeader>
          <div className='bg-muted/80 mt-2.5 h-1.5 overflow-hidden rounded-full'>
            <div
              className='bg-success h-full rounded-full'
              style={{ width: `${successPct}%` }}
            />
          </div>
        </div>

        {health.layers && health.layers.length > 0 ? (
          <div className='border-border/60 border-t px-3.5 py-3'>
            <LayeredSuccessRateList
              layers={health.layers}
              alertBelow={health.alert_below}
            />
          </div>
        ) : null}

        <div className='px-3.5 py-3'>
          {isTagRow ? (
            <TagRecentErrorsHint />
          ) : (
            <RecentErrorsPanel
              items={errorsQuery.data?.items ?? []}
              isLoading={errorsQuery.isLoading}
              isError={errorsQuery.isError}
              onRetry={() => {
                void errorsQuery.refetch()
              }}
              channelId={channel.id}
              locale={locale}
            />
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function LayeredSuccessRateList({
  layers,
  alertBelow,
}: {
  layers: ChannelHealthLayer[]
  alertBelow: number
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-2'>
      <p className='text-sm font-medium'>{t('Layered success rate')}</p>
      <ul className='flex flex-col gap-1.5'>
        {layers.map((layer) => {
          const ready = layer.sample_ready
          const variant = successRateBadgeVariant(
            layer.success_rate,
            alertBelow
          )
          const percent = ready
            ? `${Math.round(layer.success_rate * 100)}%`
            : '--'
          const fill = ready
            ? Math.min(100, Math.max(0, layer.success_rate * 100))
            : 0
          const isPrimary = layer.minutes === 5
          return (
            <li
              key={layer.minutes}
              className={cn(
                'grid grid-cols-[3.25rem_minmax(0,1fr)_2.5rem] items-center gap-2 rounded-md px-1.5 py-1',
                isPrimary && 'bg-muted/50'
              )}
            >
              <span className='text-muted-foreground text-[11px] font-medium tabular-nums'>
                {channelHealthLayerLabel(layer.minutes, t)}
              </span>
              <div className='bg-muted/80 h-1.5 overflow-hidden rounded-full'>
                <div
                  className={cn(
                    'h-full rounded-full',
                    ready ? successRateBarClass(variant) : 'bg-transparent'
                  )}
                  style={{ width: `${fill}%` }}
                />
              </div>
              <span
                className={cn(
                  'text-right text-xs font-medium tabular-nums',
                  ready ? undefined : 'text-muted-foreground'
                )}
                title={
                  ready
                    ? `${layer.success} / ${layer.total}`
                    : t('Not enough samples')
                }
              >
                {percent}
              </span>
            </li>
          )
        })}
      </ul>
      <p className='text-muted-foreground text-[11px] leading-relaxed'>
        {t(
          'The 5-minute window is used for the list badge, alerts, and auto-disable.'
        )}
      </p>
    </div>
  )
}

function TagRecentErrorsHint() {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col items-center gap-2 py-4 text-center'>
      <span className='bg-muted text-muted-foreground flex size-9 items-center justify-center rounded-full'>
        <Layers className='size-4' aria-hidden='true' />
      </span>
      <p className='text-sm font-medium'>{t('Aggregated tag rate')}</p>
      <p className='text-muted-foreground max-w-[18rem] text-xs leading-relaxed'>
        {t('Expand this tag to see recent errors for each channel.')}
      </p>
    </div>
  )
}

function RecentErrorsPanel({
  items,
  isLoading,
  isError,
  onRetry,
  channelId,
  locale,
}: {
  items: ChannelRecentErrorItem[]
  isLoading: boolean
  isError: boolean
  onRetry: () => void
  channelId: number
  locale?: string
}) {
  const { t } = useTranslation()
  let body = (
    <>
      <ul className='flex max-h-80 flex-col gap-2 overflow-y-auto pr-0.5'>
        {items.map((item) => (
          <li key={item.id}>
            <RecentErrorItemCard item={item} locale={locale} />
          </li>
        ))}
      </ul>
      <p className='text-muted-foreground text-[11px] leading-relaxed'>
        {t('Includes failures that may not count toward the success rate.')}
      </p>
    </>
  )
  if (isLoading) {
    body = <RecentErrorsLoading />
  } else if (isError) {
    body = (
      <div className='flex flex-col items-center gap-2 py-5 text-center'>
        <CircleAlert className='text-destructive size-5' aria-hidden='true' />
        <p className='text-sm'>{t('Failed to load recent errors')}</p>
        <Button variant='outline' size='xs' onClick={onRetry}>
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (items.length === 0) {
    body = (
      <div className='flex flex-col items-center gap-1.5 py-5 text-center'>
        <span className='bg-success/10 text-success flex size-9 items-center justify-center rounded-full'>
          <CircleCheck className='size-4' aria-hidden='true' />
        </span>
        <p className='text-sm font-medium'>
          {t('No error logs in the last 5 minutes')}
        </p>
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-2.5'>
      <div className='flex items-center justify-between gap-2'>
        <p className='text-sm font-medium'>{t('Recent errors')}</p>
        <Button
          variant='ghost'
          size='xs'
          className='text-muted-foreground h-6 px-1.5'
          nativeButton={false}
          render={
            <Link
              to='/usage-logs/$section'
              params={{ section: 'error' }}
              search={{ channel: String(channelId) }}
            />
          }
        >
          {t('View error logs')}
          <ArrowUpRight className='size-3' aria-hidden='true' />
        </Button>
      </div>
      {body}
    </div>
  )
}

function RecentErrorsLoading() {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-2' aria-busy='true' aria-live='polite'>
      <span className='sr-only'>{t('Loading...')}</span>
      <div className='flex items-center gap-2 py-1'>
        <Spinner className='size-3.5' />
        <span className='text-muted-foreground text-xs'>{t('Loading...')}</span>
      </div>
      <Skeleton className='h-16 w-full rounded-lg' />
      <Skeleton className='h-16 w-full rounded-lg' />
    </div>
  )
}

function RecentErrorItemCard({
  item,
  locale,
}: {
  item: ChannelRecentErrorItem
  locale?: string
}) {
  const { t } = useTranslation()
  const statusCode = item.status_code ?? 0
  const timeLabel = item.created_at
    ? formatRelativeTime(item.created_at, locale)
    : '-'
  const fullTime = formatTimestampToDate(item.created_at)

  return (
    <article className='bg-muted/40 relative rounded-lg border px-3 py-2 pl-3.5'>
      <span
        className={cn(
          'absolute inset-y-2 left-0 w-0.5 rounded-full',
          recentErrorAccentClass(statusCode)
        )}
        aria-hidden='true'
      />
      <div className='flex items-center justify-between gap-2'>
        <time
          className='text-muted-foreground text-[11px] tabular-nums'
          dateTime={
            item.created_at
              ? new Date(item.created_at * 1000).toISOString()
              : undefined
          }
          title={fullTime}
        >
          {timeLabel}
        </time>
        {statusCode > 0 ? (
          <Badge
            variant={recentErrorStatusBadgeVariant(statusCode)}
            className='h-5 px-1.5 font-mono text-[10px] tabular-nums'
          >
            {t('HTTP {{code}}', { code: statusCode })}
          </Badge>
        ) : null}
      </div>
      {item.model_name ? (
        <p className='mt-1 truncate font-mono text-[11px] font-medium'>
          {item.model_name}
        </p>
      ) : null}
      {item.content ? (
        <p className='text-muted-foreground mt-1 line-clamp-3 text-xs leading-relaxed break-words'>
          {item.content}
        </p>
      ) : null}
    </article>
  )
}
