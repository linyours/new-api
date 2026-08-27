/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Archive, Copy, Loader2, Search, Trash2, X } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { formatTimestamp } from '../channels/lib'
import { formatChannelKeyQuotaUSD } from '../channels/lib/key-quota-usd'
import {
  deleteGlobalChannelKeyArchive,
  getGlobalChannelKeyArchiveSecret,
  getGlobalChannelKeyArchives,
  restoreGlobalChannelKeyArchive,
} from './api'
import type { ArchivedChannelKey } from './types'

const PAGE_SIZE = 20

export function ChannelKeyArchives() {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canRestore = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const canCopy = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SECRET_VIEW
  )
  const canDelete = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const [page, setPage] = useState(1)
  const [channelInput, setChannelInput] = useState('')
  const [channelId, setChannelId] = useState<number | undefined>()
  const [restoreTarget, setRestoreTarget] = useState<ArchivedChannelKey | null>(
    null
  )
  const [deleteTarget, setDeleteTarget] = useState<ArchivedChannelKey | null>(
    null
  )
  const [copyingArchiveId, setCopyingArchiveId] = useState<number | null>(null)
  const { copyToClipboard } = useCopyToClipboard()
  const {
    open: verificationOpen,
    methods: verificationMethods,
    state: verificationState,
    executeVerification,
    withVerification,
    cancel: cancelVerification,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification()

  const archivesQuery = useQuery({
    queryKey: ['channel-key-archives', page, channelId],
    queryFn: () =>
      getGlobalChannelKeyArchives({
        page,
        pageSize: PAGE_SIZE,
        channelId,
      }),
    placeholderData: keepPreviousData,
  })

  const restoreMutation = useMutation({
    mutationFn: restoreGlobalChannelKeyArchive,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Operation failed'))
        return
      }
      toast.success(response.message || t('Operation successful'))
      setRestoreTarget(null)
      await archivesQuery.refetch()
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Operation failed'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteGlobalChannelKeyArchive,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Operation failed'))
        return
      }
      toast.success(response.message || t('Operation successful'))
      setDeleteTarget(null)
      if (keys.length === 1 && page > 1) {
        setPage((current) => current - 1)
        return
      }
      await archivesQuery.refetch()
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Operation failed'))
    },
  })

  const applyChannelFilter = () => {
    const value = channelInput.trim()
    if (value === '') {
      setChannelId(undefined)
      setPage(1)
      return
    }
    const parsed = Number(value)
    if (!Number.isInteger(parsed) || parsed <= 0) {
      toast.error(t('Channel ID must be a positive integer'))
      return
    }
    setChannelId(parsed)
    setPage(1)
  }

  const clearChannelFilter = () => {
    setChannelInput('')
    setChannelId(undefined)
    setPage(1)
  }

  const data = archivesQuery.data?.data
  const keys = data?.keys || []
  const totalPages = data?.total_pages || 1
  const requestFailed =
    archivesQuery.isError || archivesQuery.data?.success === false

  const fetchAndCopyArchivedKey = useCallback(
    async (archiveId: number, proofToken?: string) => {
      setCopyingArchiveId(archiveId)
      try {
        const response = await getGlobalChannelKeyArchiveSecret(
          archiveId,
          proofToken
        )
        if (!response.success || !response.data?.key) {
          throw new Error(response.message || t('Failed to fetch archived key'))
        }
        await copyToClipboard(response.data.key)
        return response
      } finally {
        setCopyingArchiveId(null)
      }
    },
    [copyToClipboard, t]
  )

  const handleCopyArchivedKey = useCallback(
    async (key: ArchivedChannelKey) => {
      try {
        await withVerification(
          (proofToken) => fetchAndCopyArchivedKey(key.archive_id, proofToken),
          {
            scope: 'channel.key.read',
            preferredMethod: 'passkey',
            title: t('Verify to copy archived key'),
            description: t(
              'Use Passkey or 2FA to confirm your identity before copying this archived key.'
            ),
          }
        )
      } catch (error) {
        if (error instanceof Error) {
          toast.error(error.message)
        }
      }
    },
    [fetchAndCopyArchivedKey, t, withVerification]
  )

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          <span className='flex items-center gap-2'>
            <Archive className='h-5 w-5' aria-hidden='true' />
            {t('Exhausted key archive')}
            <Badge variant='outline'>{data?.total || 0}</Badge>
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <p className='text-muted-foreground shrink-0 text-sm'>
              {t(
                'Manage exhausted keys across all channels and restore them into a new quota cycle.'
              )}
            </p>
            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              <Input
                type='number'
                min={1}
                className='w-48'
                value={channelInput}
                onChange={(event) => setChannelInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') applyChannelFilter()
                }}
                placeholder={t('Channel ID')}
                aria-label={t('Channel ID')}
              />
              <Button
                type='button'
                variant='outline'
                onClick={applyChannelFilter}
              >
                <Search className='h-4 w-4' aria-hidden='true' />
                {t('Filter')}
              </Button>
              {channelId !== undefined && (
                <Button
                  type='button'
                  variant='ghost'
                  onClick={clearChannelFilter}
                >
                  <X className='h-4 w-4' aria-hidden='true' />
                  {t('Clear')}
                </Button>
              )}
            </div>

            <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
              {archivesQuery.isLoading && (
                <div className='flex items-center justify-center py-16'>
                  <Loader2
                    className='text-muted-foreground h-8 w-8 animate-spin'
                    aria-hidden='true'
                  />
                </div>
              )}
              {!archivesQuery.isLoading && requestFailed && (
                <div className='text-destructive py-16 text-center'>
                  {archivesQuery.data?.message ||
                    t('Failed to load exhausted key archive')}
                </div>
              )}
              {!archivesQuery.isLoading &&
                !requestFailed &&
                keys.length === 0 && (
                  <div className='text-muted-foreground py-16 text-center'>
                    {t('No archived keys found')}
                  </div>
                )}
              {!archivesQuery.isLoading &&
                !requestFailed &&
                keys.length > 0 && (
                  <StaticDataTable
                    className='rounded-none border-0'
                    tableClassName='min-w-[980px]'
                    data={keys}
                    getRowKey={(key) => key.archive_id}
                    columns={[
                      {
                        id: 'channel',
                        header: t('Channel'),
                        cell: (key) => (
                          <div>
                            <div className='font-medium'>
                              {key.channel_name || `#${key.channel_id}`}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              #{key.channel_id}
                            </div>
                          </div>
                        ),
                      },
                      {
                        id: 'key',
                        header: t('Key'),
                        cellClassName: 'font-mono text-sm',
                        cell: (key) => key.key_preview,
                      },
                      {
                        id: 'quota',
                        header: t('USD usage'),
                        cellClassName: 'font-mono text-sm',
                        cell: (key) =>
                          `${formatChannelKeyQuotaUSD(key.quota_used)} / ${formatChannelKeyQuotaUSD(key.effective_quota)}`,
                      },
                      {
                        id: 'lifetime',
                        header: t('Lifetime USD spend'),
                        cellClassName: 'font-mono text-sm',
                        cell: (key) =>
                          formatChannelKeyQuotaUSD(key.lifetime_quota),
                      },
                      {
                        id: 'reason',
                        header: t('Reason'),
                        cellClassName: 'max-w-64 truncate text-sm',
                        cell: (key) => key.reason || '-',
                      },
                      {
                        id: 'archived-at',
                        header: t('Archived at'),
                        cellClassName: 'text-muted-foreground text-sm',
                        cell: (key) => formatTimestamp(key.archived_time),
                      },
                      {
                        id: 'actions',
                        header: t('Actions'),
                        className: 'text-right',
                        cell: (key) => (
                          <div className='flex justify-end gap-1'>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => handleCopyArchivedKey(key)}
                              disabled={
                                !canCopy ||
                                copyingArchiveId === key.archive_id ||
                                verificationState.loading
                              }
                              title={
                                canCopy
                                  ? t('Copy key')
                                  : t('No permission to perform this action')
                              }
                              aria-label={t('Copy key')}
                            >
                              {copyingArchiveId === key.archive_id ? (
                                <Loader2
                                  className='h-4 w-4 animate-spin'
                                  aria-hidden='true'
                                />
                              ) : (
                                <Copy className='h-4 w-4' aria-hidden='true' />
                              )}
                            </Button>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              onClick={() => setRestoreTarget(key)}
                              disabled={!canRestore}
                              title={
                                canRestore
                                  ? undefined
                                  : t('No permission to perform this action')
                              }
                            >
                              {t('Restore')}
                            </Button>
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              className='text-destructive hover:text-destructive'
                              onClick={() => setDeleteTarget(key)}
                              disabled={!canDelete}
                              title={
                                canDelete
                                  ? t('Delete')
                                  : t('No permission to perform this action')
                              }
                              aria-label={t('Delete')}
                            >
                              <Trash2 className='h-4 w-4' aria-hidden='true' />
                            </Button>
                          </div>
                        ),
                      },
                    ]}
                  />
                )}
            </div>

            <div className='flex shrink-0 items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Page {{current}} of {{total}}', {
                  current: data?.page || page,
                  total: totalPages,
                })}
              </span>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setPage((current) => current - 1)}
                  disabled={page <= 1 || archivesQuery.isFetching}
                >
                  {t('Previous')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setPage((current) => current + 1)}
                  disabled={page >= totalPages || archivesQuery.isFetching}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ConfirmDialog
        open={restoreTarget !== null}
        onOpenChange={(open) => !open && setRestoreTarget(null)}
        title={t('Restore archived key?')}
        desc={
          <div className='space-y-2'>
            <p>
              {t(
                'Restoring resets the current usage and enables this key for scheduling again.'
              )}
            </p>
            <p className='text-foreground font-medium'>
              {t(
                'A new quota cycle will begin. When usage reaches {{limit}}, the key will be archived again.',
                {
                  limit: formatChannelKeyQuotaUSD(
                    restoreTarget?.effective_quota || 0
                  ),
                }
              )}
            </p>
          </div>
        }
        confirmText={t('Restore')}
        isLoading={restoreMutation.isPending}
        handleConfirm={() => {
          if (restoreTarget) {
            restoreMutation.mutate(restoreTarget.archive_id)
          }
        }}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Permanently delete archived key?')}
        desc={t(
          'This permanently removes the key from its channel and deletes its archived secret. This action cannot be undone.'
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget.archive_id)
          }
        }}
      />
      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          if (!open) {
            cancelVerification()
          }
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </>
  )
}
