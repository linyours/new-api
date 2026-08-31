/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChartColumn, Loader2, Search, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { getChannelCreatorUsage } from './api'
import { CreatorUsageDetailDialog } from './components/creator-usage-detail-dialog'
import {
  formatCreatorDisplayName,
  formatQuotaRatio,
} from './lib/format'
import type { ChannelCreatorUsageItem } from './types'

const PAGE_SIZE = 20

export function ChannelCreatorUsage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [selectedCreator, setSelectedCreator] =
    useState<ChannelCreatorUsageItem | null>(null)

  const usageQuery = useQuery({
    queryKey: ['channel-creator-usage', page, keyword],
    queryFn: () =>
      getChannelCreatorUsage({
        page,
        pageSize: PAGE_SIZE,
        keyword,
      }),
    placeholderData: keepPreviousData,
  })

  const data = usageQuery.data?.data
  const items = data?.items ?? []
  const total = data?.total ?? 0
  const summary = data?.summary
  const requestFailed = usageQuery.isError || usageQuery.data?.success === false
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const applyKeyword = () => {
    setPage(1)
    setKeyword(keywordInput.trim())
  }

  const clearKeyword = () => {
    setKeywordInput('')
    setKeyword('')
    setPage(1)
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          <span className='flex items-center gap-2'>
            <ChartColumn className='h-5 w-5' aria-hidden='true' />
            {t('Channel creator usage')}
            <Badge variant='outline'>{summary?.creator_count || 0}</Badge>
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <p className='text-muted-foreground shrink-0 text-sm'>
              {t(
                'Lifetime consumption grouped by the admin who created each channel. Values come from channel used_quota.'
              )}
            </p>

            <div className='grid shrink-0 gap-3 sm:grid-cols-3'>
              <SummaryCard
                label={t('Total usage')}
                value={formatQuotaWithCurrency(summary?.total_used_quota ?? 0, {
                  digitsLarge: 2,
                  digitsSmall: 4,
                  abbreviate: false,
                })}
              />
              <SummaryCard
                label={t('Total channels')}
                value={String(summary?.total_channels ?? 0)}
              />
              <SummaryCard
                label={t('Unknown creator channels')}
                value={String(summary?.unknown_creator_channels ?? 0)}
              />
            </div>

            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              <Input
                className='w-64'
                value={keywordInput}
                onChange={(event) => setKeywordInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') applyKeyword()
                }}
                placeholder={t('Search by username or user ID')}
                aria-label={t('Search by username or user ID')}
              />
              <Button type='button' variant='outline' onClick={applyKeyword}>
                <Search className='h-4 w-4' aria-hidden='true' />
                {t('Filter')}
              </Button>
              {keyword && (
                <Button type='button' variant='ghost' onClick={clearKeyword}>
                  <X className='h-4 w-4' aria-hidden='true' />
                  {t('Clear')}
                </Button>
              )}
            </div>

            <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
              {usageQuery.isLoading && (
                <div className='flex items-center justify-center py-16'>
                  <Loader2
                    className='text-muted-foreground h-8 w-8 animate-spin'
                    aria-hidden='true'
                  />
                </div>
              )}
              {!usageQuery.isLoading && requestFailed && (
                <div className='text-destructive py-16 text-center'>
                  {usageQuery.data?.message ||
                    t('Failed to load channel creator usage')}
                </div>
              )}
              {!usageQuery.isLoading && !requestFailed && items.length === 0 && (
                <div className='text-muted-foreground py-16 text-center'>
                  {t('No creator usage data found')}
                </div>
              )}
              {!usageQuery.isLoading && !requestFailed && items.length > 0 && (
                <StaticDataTable
                  className='rounded-none border-0'
                  tableClassName='min-w-[960px]'
                  data={items}
                  getRowKey={(item) => item.created_by}
                  columns={[
                    {
                      id: 'creator',
                      header: t('Creator'),
                      cell: (item) => (
                        <div>
                          <div className='font-medium'>
                            {formatCreatorDisplayName(
                              item.created_by,
                              item.username,
                              t
                            )}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            #{item.created_by}
                          </div>
                        </div>
                      ),
                    },
                    {
                      id: 'channels',
                      header: t('Channels'),
                      cell: (item) => (
                        <div className='text-sm'>
                          <div>
                            {item.channel_count} ({t('Enabled')}:{' '}
                            {item.enabled_count})
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {t('Disabled')}: {item.disabled_count}
                          </div>
                        </div>
                      ),
                    },
                    {
                      id: 'used_quota',
                      header: t('Usage'),
                      cellClassName: 'font-mono text-sm',
                      cell: (item) =>
                        formatQuotaWithCurrency(item.used_quota, {
                          digitsLarge: 2,
                          digitsSmall: 4,
                          abbreviate: false,
                        }),
                    },
                    {
                      id: 'ratio',
                      header: t('Share of total'),
                      cellClassName: 'font-mono text-sm',
                      cell: (item) => formatQuotaRatio(item.quota_ratio),
                    },
                    {
                      id: 'actions',
                      header: t('Actions'),
                      className: 'text-right',
                      cell: (item) => (
                        <div className='flex justify-end'>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            onClick={() => setSelectedCreator(item)}
                          >
                            {t('View details')}
                          </Button>
                        </div>
                      ),
                    },
                  ]}
                />
              )}
            </div>

            {totalPages > 1 && (
              <div className='flex shrink-0 items-center justify-end gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  {t('Previous')}
                </Button>
                <span className='text-muted-foreground text-sm'>
                  {page} / {totalPages}
                </span>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() =>
                    setPage((current) => Math.min(totalPages, current + 1))
                  }
                >
                  {t('Next')}
                </Button>
              </div>
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CreatorUsageDetailDialog
        creator={selectedCreator}
        open={selectedCreator !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedCreator(null)
        }}
      />
    </>
  )
}

function SummaryCard(props: { label: string; value: string }) {
  return (
    <div className='bg-card rounded-lg border p-4'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-lg font-semibold'>{props.value}</div>
    </div>
  )
}
