/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteChannelTemplate,
  getChannelTemplates,
  updateChannelTemplate,
} from '../../api'
import { SUCCESS_MESSAGES } from '../../constants'
import { channelsQueryKeys, getChannelTypeLabel } from '../../lib'
import type { ChannelTemplate } from '../../types'
import { useChannels } from '../channels-provider'

type ManageChannelTemplatesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ManageChannelTemplatesDialog({
  open,
  onOpenChange,
}: ManageChannelTemplatesDialogProps) {
  const { t } = useTranslation()
  const { setApplyTemplateId, setOpen } = useChannels()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canApply = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const [deleteTarget, setDeleteTarget] = useState<ChannelTemplate | null>(null)
  const [savingId, setSavingId] = useState<number | null>(null)
  const [drafts, setDrafts] = useState<Record<number, string>>({})

  const templatesQuery = useQuery({
    queryKey: channelsQueryKeys.templates(),
    queryFn: getChannelTemplates,
    enabled: open,
  })
  const templates = templatesQuery.data?.data || []

  const handleRename = async (template: ChannelTemplate) => {
    const nextName = (drafts[template.id] ?? template.name).trim()
    if (!nextName || nextName === template.name) return
    setSavingId(template.id)
    try {
      const response = await updateChannelTemplate({
        id: template.id,
        name: nextName,
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to update channel template'))
        return
      }
      toast.success(t(SUCCESS_MESSAGES.TEMPLATE_UPDATED))
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.templates(),
      })
    } catch {
      toast.error(t('Failed to update channel template'))
    } finally {
      setSavingId(null)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      const response = await deleteChannelTemplate(deleteTarget.id)
      if (!response.success) {
        toast.error(response.message || t('Failed to delete channel template'))
        return
      }
      toast.success(t(SUCCESS_MESSAGES.TEMPLATE_DELETED))
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.templates(),
      })
      setDeleteTarget(null)
    } catch {
      toast.error(t('Failed to delete channel template'))
    }
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={t('Channel Templates')}
        description={t(
          'Templates store channel settings without keys. Use a template to create new channels from a key list.'
        )}
        contentHeight='auto'
        bodyClassName='space-y-3'
        footer={
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
        }
      >
        <div className='space-y-3 py-2'>
          {templatesQuery.isLoading ? (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Loader2 className='h-4 w-4 animate-spin' />
              {t('Loading...')}
            </div>
          ) : null}
          {!templatesQuery.isLoading && templates.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t(
                'No channel templates yet. Save a configured channel as a template first.'
              )}
            </p>
          ) : null}
          {templates.map((template) => (
            <div
              key={template.id}
              className='flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-center'
            >
              <div className='min-w-0 flex-1 space-y-1'>
                <Input
                  maxLength={64}
                  value={drafts[template.id] ?? template.name}
                  onChange={(event) =>
                    setDrafts((current) => ({
                      ...current,
                      [template.id]: event.target.value,
                    }))
                  }
                  onBlur={() => handleRename(template)}
                  disabled={savingId === template.id}
                />
                <p className='text-muted-foreground truncate text-xs'>
                  {t(getChannelTypeLabel(template.channel_type))}
                  {template.description ? ` · ${template.description}` : ''}
                </p>
              </div>
              <div className='flex shrink-0 gap-2'>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  disabled={!canApply}
                  onClick={() => {
                    if (!canApply) return
                    setApplyTemplateId(template.id)
                    onOpenChange(false)
                    setOpen('apply-template')
                  }}
                >
                  {t('Use')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  variant='ghost'
                  onClick={() => setDeleteTarget(template)}
                  aria-label={t('Delete')}
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              </div>
            </div>
          ))}
        </div>
      </Dialog>
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setDeleteTarget(null)
        }}
        title={t('Delete Template')}
        desc={t(
          'Delete template "{{name}}"? Existing channels are not affected.',
          { name: deleteTarget?.name ?? '' }
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={handleDelete}
      />
    </>
  )
}
