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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers, Loader2, Plus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  getChannelTemplates,
  getStandbyPool,
  getStandbyPoolChannels,
  standbyPoolQueryKey,
  updateStandbyPool,
} from '@/features/channels/api'
import { ApplyChannelTemplateDialog } from '@/features/channels/components/dialogs/apply-channel-template-dialog'
import { CHANNEL_STATUS_CONFIG } from '@/features/channels/constants'
import { channelsQueryKeys, getChannelTypeLabel } from '@/features/channels/lib'
import type { StandbyPoolChannel } from '@/features/channels/types'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const MAX_STANDBY_PROMOTE_THRESHOLD = 100000
const MAX_STANDBY_PROMOTE_BATCH = 500
const POOL_CHANNEL_PAGE_SIZE = 20

type PromoteDraft = {
  enabled: boolean
  threshold: string
  batch: string
}

function parseWholeNumber(value: string): number | null {
  const trimmed = value.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const parsed = Number(trimmed)
  if (!Number.isSafeInteger(parsed)) return null
  return parsed
}

export function StandbyPoolPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canImport = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const canSaveRule = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const [importOpen, setImportOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState<PromoteDraft | null>(null)
  const [channelPage, setChannelPage] = useState(1)

  const poolQuery = useQuery({
    queryKey: standbyPoolQueryKey,
    queryFn: getStandbyPool,
  })
  const pool = poolQuery.data?.data
  const templatesQuery = useQuery({
    queryKey: channelsQueryKeys.templates(),
    queryFn: getChannelTemplates,
  })
  const hasTemplates = (templatesQuery.data?.data?.length ?? 0) > 0
  const channelsQuery = useQuery({
    queryKey: [...standbyPoolQueryKey, 'channels', channelPage],
    queryFn: () =>
      getStandbyPoolChannels({
        p: channelPage,
        page_size: POOL_CHANNEL_PAGE_SIZE,
      }),
  })
  const poolChannels = channelsQuery.data?.data?.items ?? []
  const poolChannelTotal = channelsQuery.data?.data?.total ?? 0
  const channelPageCount = Math.max(
    1,
    Math.ceil(poolChannelTotal / POOL_CHANNEL_PAGE_SIZE)
  )

  useEffect(() => {
    if (channelsQuery.isLoading || channelsQuery.isFetching) return
    if (channelPage > channelPageCount) setChannelPage(channelPageCount)
  }, [
    channelPage,
    channelPageCount,
    channelsQuery.isFetching,
    channelsQuery.isLoading,
  ])

  useEffect(() => {
    if (!pool) return
    setDraft((current) => {
      if (current) return current
      return {
        enabled: pool.promote_enabled,
        threshold: String(pool.promote_threshold ?? 0),
        batch: String(pool.promote_batch ?? 0),
      }
    })
  }, [pool])

  const handleSaveRule = async () => {
    if (!canSaveRule || !draft) return
    const threshold = parseWholeNumber(draft.threshold)
    const batch = parseWholeNumber(draft.batch)
    if (threshold === null || threshold > MAX_STANDBY_PROMOTE_THRESHOLD) {
      toast.error(
        t('Enter a whole number from {{min}} to {{max}}', {
          min: 0,
          max: MAX_STANDBY_PROMOTE_THRESHOLD,
        })
      )
      return
    }
    const minBatch = draft.enabled ? 1 : 0
    if (
      batch === null ||
      batch < minBatch ||
      batch > MAX_STANDBY_PROMOTE_BATCH
    ) {
      toast.error(
        t('Enter a whole number from {{min}} to {{max}}', {
          min: minBatch,
          max: MAX_STANDBY_PROMOTE_BATCH,
        })
      )
      return
    }

    setSaving(true)
    try {
      const response = await updateStandbyPool({
        promote_enabled: draft.enabled,
        promote_threshold: threshold,
        promote_batch: batch,
      })
      if (!response.success || !response.data) {
        toast.error(
          response.message || t('Failed to save the standby pool rule')
        )
        return
      }
      setDraft({
        enabled: response.data.promote_enabled,
        threshold: String(response.data.promote_threshold ?? 0),
        batch: String(response.data.promote_batch ?? 0),
      })
      toast.success(t('Promotion rule saved'))
      queryClient.invalidateQueries({ queryKey: standbyPoolQueryKey })
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    } catch {
      toast.error(t('Failed to save the standby pool rule'))
    } finally {
      setSaving(false)
    }
  }

  const ruleDraft = draft ?? {
    enabled: pool?.promote_enabled ?? false,
    threshold: String(pool?.promote_threshold ?? 0),
    batch: String(pool?.promote_batch ?? 0),
  }
  const threshold = parseWholeNumber(ruleDraft.threshold)
  const batch = parseWholeNumber(ruleDraft.batch)
  const ruleValid =
    threshold !== null &&
    threshold <= MAX_STANDBY_PROMOTE_THRESHOLD &&
    batch !== null &&
    batch <= MAX_STANDBY_PROMOTE_BATCH &&
    (!ruleDraft.enabled || batch >= 1)
  const dirty =
    pool != null &&
    (ruleDraft.enabled !== pool.promote_enabled ||
      ruleDraft.threshold !== String(pool.promote_threshold ?? 0) ||
      ruleDraft.batch !== String(pool.promote_batch ?? 0))
  const enabledCount = pool?.enabled_count ?? 0
  const standbyCount = pool?.standby_count ?? 0
  const unavailableCount = pool?.unavailable_count ?? 0
  const belowLine =
    ruleDraft.enabled &&
    threshold !== null &&
    enabledCount <= threshold &&
    standbyCount === 0
  let ruleText = t(
    'Automatic promotion is off. Keys added to this pool stay on standby.'
  )
  if (belowLine) {
    ruleText = t('The standby pool is empty, so nothing can be promoted.')
  } else if (ruleDraft.enabled && ruleValid) {
    ruleText = t(
      'When enabled channels drop to {{threshold}} or below, promote {{batch}} standby channels.',
      { threshold, batch }
    )
  } else if (ruleDraft.enabled) {
    ruleText = t('Enter a whole number from {{min}} to {{max}}', {
      min: 1,
      max: MAX_STANDBY_PROMOTE_BATCH,
    })
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          <span className='flex items-center gap-2'>
            <Layers className='h-5 w-5' aria-hidden='true' />
            {t('Standby pool')}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            disabled={!canImport || !hasTemplates}
            onClick={() => {
              if (!canImport) return
              setImportOpen(true)
            }}
          >
            <Plus data-icon='inline-start' />
            {t('Import keys')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex w-full min-w-0 flex-col gap-4'>
            {poolQuery.isLoading ? (
              <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                <Loader2 className='h-4 w-4 animate-spin' />
                {t('Loading...')}
              </div>
            ) : null}

            {poolQuery.isError ? (
              <p className='text-destructive text-sm'>{t('Failed to load')}</p>
            ) : null}

            {pool ? (
              <div className='flex w-full min-w-0 flex-col gap-4'>
                <div className='grid w-full min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)]'>
                  <div className='flex min-w-0 flex-col gap-4'>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Import keys with a channel template. Every key enters this one standby pool.'
                      )}
                    </p>
                    <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
                      <PoolStat
                        label={t('Enabled channels')}
                        value={enabledCount}
                        tone='success'
                      />
                      <PoolStat
                        label={t('Standby channels')}
                        value={standbyCount}
                        tone='info'
                      />
                      <PoolStat
                        label={t('Unavailable channels')}
                        value={unavailableCount}
                        tone='warning'
                      />
                    </div>
                  </div>

                  <section className='bg-card min-w-0 rounded-xl border p-4 shadow-sm'>
                    <PoolRule
                      draft={ruleDraft}
                      canSaveRule={canSaveRule}
                      dirty={dirty}
                      ruleValid={ruleValid}
                      saving={saving}
                      ruleText={ruleText}
                      belowLine={belowLine}
                      onDraftChange={setDraft}
                      onSave={handleSaveRule}
                    />
                    {!templatesQuery.isLoading && !hasTemplates ? (
                      <p className='text-muted-foreground mt-3 text-sm'>
                        {t(
                          'No channel templates yet. Save a configured channel as a template, then import keys here.'
                        )}
                      </p>
                    ) : null}
                  </section>
                </div>

                <PoolChannelList
                  channels={poolChannels}
                  total={poolChannelTotal}
                  page={channelPage}
                  pageCount={channelPageCount}
                  loading={channelsQuery.isLoading}
                  failed={channelsQuery.isError}
                  onPageChange={setChannelPage}
                />
              </div>
            ) : null}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApplyChannelTemplateDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        mode='standby'
      />
    </>
  )
}

function PoolRule(props: {
  draft: PromoteDraft
  canSaveRule: boolean
  dirty: boolean
  ruleValid: boolean
  saving: boolean
  ruleText: string
  belowLine: boolean
  onDraftChange: (draft: PromoteDraft) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const { draft } = props
  return (
    <div className='space-y-3'>
      <div className='flex items-start justify-between gap-3'>
        <div className='space-y-1'>
          <Label htmlFor='standby-switch'>{t('Auto promote')}</Label>
          <p className='text-muted-foreground text-xs'>
            {t('Promote standby channels when this pool runs low.')}
          </p>
        </div>
        <Switch
          id='standby-switch'
          checked={draft.enabled}
          disabled={!props.canSaveRule}
          onCheckedChange={(checked) => {
            const nextBatch =
              checked && (parseWholeNumber(draft.batch) ?? 0) < 1
                ? '20'
                : draft.batch
            props.onDraftChange({
              ...draft,
              enabled: checked,
              batch: nextBatch,
            })
          }}
        />
      </div>

      {draft.enabled ? (
        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-1'>
          <div className='space-y-1.5'>
            <Label htmlFor='standby-threshold'>
              {t('Enabled count at or below')}
            </Label>
            <Input
              id='standby-threshold'
              inputMode='numeric'
              value={draft.threshold}
              disabled={!props.canSaveRule}
              onChange={(event) =>
                props.onDraftChange({
                  ...draft,
                  threshold: event.target.value,
                })
              }
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='standby-batch'>
              {t('Channels promoted each time')}
            </Label>
            <Input
              id='standby-batch'
              inputMode='numeric'
              value={draft.batch}
              disabled={!props.canSaveRule}
              onChange={(event) =>
                props.onDraftChange({
                  ...draft,
                  batch: event.target.value,
                })
              }
            />
          </div>
        </div>
      ) : null}

      <p
        className={cn(
          'text-xs',
          props.belowLine ? 'text-warning-foreground' : 'text-muted-foreground'
        )}
      >
        {props.ruleText}
      </p>

      <div className='flex justify-end'>
        <Button
          type='button'
          size='sm'
          disabled={
            !props.canSaveRule ||
            !props.dirty ||
            !props.ruleValid ||
            props.saving
          }
          onClick={props.onSave}
        >
          {props.saving ? (
            <Loader2 className='mr-2 h-4 w-4 animate-spin' />
          ) : null}
          {t('Save rule')}
        </Button>
      </div>
    </div>
  )
}

function PoolChannelList(props: {
  channels: StandbyPoolChannel[]
  total: number
  page: number
  pageCount: number
  loading: boolean
  failed: boolean
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <section className='bg-card min-w-0 rounded-xl border shadow-sm'>
      <div className='flex items-center justify-between gap-3 border-b px-4 py-3'>
        <h3 className='text-sm font-medium'>{t('Channels in this pool')}</h3>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {props.total}
        </span>
      </div>

      {props.loading ? (
        <div className='text-muted-foreground flex items-center gap-2 px-4 py-8 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Loading...')}
        </div>
      ) : null}

      {props.failed ? (
        <p className='text-destructive px-4 py-8 text-sm'>
          {t('Failed to load')}
        </p>
      ) : null}

      {!props.loading && !props.failed && props.channels.length === 0 ? (
        <p className='text-muted-foreground px-4 py-8 text-sm'>
          {t('No channels in this standby pool yet.')}
        </p>
      ) : null}

      {!props.loading && !props.failed && props.channels.length > 0 ? (
        <div className='overflow-x-auto'>
          <table className='w-full min-w-[36rem] text-sm'>
            <thead className='text-muted-foreground border-b text-left text-xs'>
              <tr>
                <th className='px-4 py-2 font-medium'>{t('Name')}</th>
                <th className='px-4 py-2 font-medium'>{t('Type')}</th>
                <th className='px-4 py-2 font-medium'>{t('Template')}</th>
                <th className='px-4 py-2 font-medium'>{t('Group')}</th>
                <th className='px-4 py-2 font-medium'>{t('Models')}</th>
                <th className='px-4 py-2 font-medium'>{t('Status')}</th>
              </tr>
            </thead>
            <tbody>
              {props.channels.map((channel) => {
                const status =
                  CHANNEL_STATUS_CONFIG[
                    channel.status as keyof typeof CHANNEL_STATUS_CONFIG
                  ] ?? CHANNEL_STATUS_CONFIG[0]
                return (
                  <tr key={channel.id} className='border-b last:border-b-0'>
                    <td className='max-w-48 truncate px-4 py-2.5 font-medium'>
                      {channel.name}
                    </td>
                    <td className='text-muted-foreground px-4 py-2.5 whitespace-nowrap'>
                      {t(getChannelTypeLabel(channel.type))}
                    </td>
                    <td className='text-muted-foreground max-w-40 truncate px-4 py-2.5'>
                      {channel.template_name || '—'}
                    </td>
                    <td className='text-muted-foreground max-w-32 truncate px-4 py-2.5'>
                      {channel.group || '—'}
                    </td>
                    <td
                      className='text-muted-foreground max-w-64 truncate px-4 py-2.5'
                      title={channel.models}
                    >
                      {channel.models || '—'}
                    </td>
                    <td className='px-4 py-2.5'>
                      <StatusBadge
                        label={t(status.label)}
                        variant={status.variant}
                        size='sm'
                        copyable={false}
                      />
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      {props.pageCount > 1 ? (
        <div className='flex items-center justify-end gap-2 border-t px-4 py-3'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={props.page <= 1}
            onClick={() => props.onPageChange(props.page - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {props.page}/{props.pageCount}
          </span>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={props.page >= props.pageCount}
            onClick={() => props.onPageChange(props.page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}

function PoolStat(props: {
  label: string
  value: number
  tone: 'success' | 'info' | 'warning'
}) {
  return (
    <div
      className={cn(
        'rounded-lg border px-3 py-2',
        props.tone === 'success' && 'border-success/30 bg-success/8',
        props.tone === 'info' && 'border-info/30 bg-info/8',
        props.tone === 'warning' && 'border-warning/30 bg-warning/10'
      )}
    >
      <div className='text-muted-foreground text-[11px] leading-4'>
        {props.label}
      </div>
      <div className='mt-1 text-lg font-semibold tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}
