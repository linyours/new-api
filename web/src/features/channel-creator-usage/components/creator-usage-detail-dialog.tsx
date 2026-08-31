/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useQuery } from '@tanstack/react-query'
import { Download, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { formatTimestamp } from '../../channels/lib'
import {
  downloadChannelCreatorUsageExport,
  getChannelCreatorUsageChannels,
} from '../api'
import type { ChannelCreatorUsageItem } from '../types'
import { formatCreatorDisplayName, formatQuotaRatio } from '../lib/format'

function channelStatusLabel(
  status: number,
  t: (key: string) => string
): string {
  if (status === 1) return t('Enabled')
  if (status === 2) return t('Manually disabled')
  if (status === 3) return t('Auto disabled')
  return t('Unknown')
}

type CreatorUsageDetailDialogProps = {
  creator: ChannelCreatorUsageItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreatorUsageDetailDialog(
  props: CreatorUsageDetailDialogProps
) {
  const { t } = useTranslation()
  const [downloading, setDownloading] = useState(false)
  const creatorId = props.creator?.created_by
  const detailQuery = useQuery({
    queryKey: ['channel-creator-usage-detail', creatorId],
    queryFn: () => getChannelCreatorUsageChannels(creatorId ?? 0),
    enabled: props.open && creatorId !== undefined,
  })

  const detail = detailQuery.data?.data
  const channels = detail?.channels ?? []
  const displayName = formatCreatorDisplayName(
    props.creator?.created_by ?? 0,
    detail?.username || props.creator?.username || '',
    t
  )

  const handleDownload = async () => {
    if (creatorId === undefined) return
    setDownloading(true)
    try {
      const blob = await downloadChannelCreatorUsageExport(creatorId)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `channel-creator-usage-${creatorId}.xlsx`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
      toast.success(t('Download started'))
    } catch {
      toast.error(t('Failed to download reconciliation file'))
    } finally {
      setDownloading(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex max-h-[90vh] max-w-5xl flex-col gap-4 overflow-hidden sm:max-w-5xl'>
        <DialogHeader>
          <DialogTitle>{t('Creator channel usage')}</DialogTitle>
          <DialogDescription>
            {t(
              'Lifetime consumption for every channel created by this user. Amounts come from each channel used_quota.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-wrap items-center gap-2 text-sm'>
          <span className='font-medium'>{displayName}</span>
          <Badge variant='outline'>
            {t('Channels')}: {detail?.channel_count ?? props.creator?.channel_count ?? 0}
          </Badge>
          <Badge variant='secondary'>
            {t('Total usage')}:{' '}
            {formatQuotaWithCurrency(
              detail?.used_quota ?? props.creator?.used_quota ?? 0,
              { digitsLarge: 2, digitsSmall: 4, abbreviate: false }
            )}
          </Badge>
        </div>

        <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
          {detailQuery.isLoading && (
            <div className='flex items-center justify-center py-16'>
              <Loader2
                className='text-muted-foreground h-8 w-8 animate-spin'
                aria-hidden='true'
              />
            </div>
          )}
          {!detailQuery.isLoading && detailQuery.isError && (
            <div className='text-destructive py-16 text-center'>
              {t('Failed to load creator channel usage')}
            </div>
          )}
          {!detailQuery.isLoading &&
            !detailQuery.isError &&
            channels.length === 0 && (
              <div className='text-muted-foreground py-16 text-center'>
                {t('No channels found for this creator')}
              </div>
            )}
          {!detailQuery.isLoading &&
            !detailQuery.isError &&
            channels.length > 0 && (
              <StaticDataTable
                className='rounded-none border-0'
                tableClassName='min-w-[880px]'
                data={channels}
                getRowKey={(channel) => channel.id}
                columns={[
                  {
                    id: 'id',
                    header: t('ID'),
                    cellClassName: 'font-mono text-sm',
                    cell: (channel) => channel.id,
                  },
                  {
                    id: 'name',
                    header: t('Name'),
                    cell: (channel) => (
                      <div>
                        <div className='font-medium'>{channel.name}</div>
                        <div className='text-muted-foreground text-xs'>
                          {channel.type_name || `#${channel.type}`}
                        </div>
                      </div>
                    ),
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    cell: (channel) =>
                      channelStatusLabel(channel.status, t),
                  },
                  {
                    id: 'group',
                    header: t('Group'),
                    cell: (channel) => channel.group || '-',
                  },
                  {
                    id: 'used_quota',
                    header: t('Usage'),
                    cellClassName: 'font-mono text-sm',
                    cell: (channel) =>
                      formatQuotaWithCurrency(channel.used_quota, {
                        digitsLarge: 2,
                        digitsSmall: 4,
                        abbreviate: false,
                      }),
                  },
                  {
                    id: 'created_time',
                    header: t('Created at'),
                    cellClassName: 'text-muted-foreground text-sm',
                    cell: (channel) => formatTimestamp(channel.created_time),
                  },
                ]}
              />
            )}
        </div>

        <DialogFooter className='gap-2 sm:justify-between'>
          <div className='text-muted-foreground text-sm'>
            {t('Share of total')}:{' '}
            {formatQuotaRatio(props.creator?.quota_ratio ?? 0)}
          </div>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
            >
              {t('Close')}
            </Button>
            <Button
              type='button'
              onClick={handleDownload}
              disabled={downloading || channels.length === 0}
            >
              {downloading ? (
                <Loader2 className='h-4 w-4 animate-spin' aria-hidden='true' />
              ) : (
                <Download className='h-4 w-4' aria-hidden='true' />
              )}
              {t('Download for reconciliation')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
