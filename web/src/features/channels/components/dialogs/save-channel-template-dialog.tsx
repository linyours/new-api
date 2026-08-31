/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { createChannelTemplateFromChannel } from '../../api'
import { SUCCESS_MESSAGES } from '../../constants'
import {
  DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  channelsQueryKeys,
} from '../../lib'
import { useChannels } from '../channels-provider'

type SaveChannelTemplateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SaveChannelTemplateDialog({
  open,
  onOpenChange,
}: SaveChannelTemplateDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [suffixLength, setSuffixLength] = useState(
    DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  )
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    if (!open || !currentRow) return
    setName(currentRow.name)
    setDescription('')
    setSuffixLength(DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH)
  }, [open, currentRow])

  if (!currentRow) return null

  const handleSave = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) {
      toast.error(t('Template name is required'))
      return
    }
    setIsSaving(true)
    try {
      const response = await createChannelTemplateFromChannel(currentRow.id, {
        name: trimmedName,
        description: description.trim(),
        name_suffix_length: suffixLength,
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to save channel template'))
        return
      }
      toast.success(t(SUCCESS_MESSAGES.TEMPLATE_SAVED))
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.templates(),
      })
      onOpenChange(false)
    } catch {
      toast.error(t('Failed to save channel template'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Save as Template')}
      description={t(
        "Save this channel's settings as a reusable template. Keys are not stored. Later you can paste new keys to create channels named with the key suffix."
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSaving}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isSaving}>
            {isSaving && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Save Template')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-2'>
        <div className='space-y-2'>
          <Label htmlFor='template-name'>{t('Template Name')}</Label>
          <Input
            id='template-name'
            maxLength={64}
            value={name}
            onChange={(event) => setName(event.target.value)}
            disabled={isSaving}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'New channels will be named like {{example}}',
              { example: `${trimmedPreviewName(name)}-abcdef` }
            )}
          </p>
        </div>
        <div className='space-y-2'>
          <Label htmlFor='template-description'>{t('Description')}</Label>
          <Textarea
            id='template-description'
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            disabled={isSaving}
            rows={3}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='template-suffix-length'>
            {t('Key suffix length')}
          </Label>
          <Input
            id='template-suffix-length'
            type='number'
            min={MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH}
            max={MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH}
            value={suffixLength}
            onChange={(event) =>
              setSuffixLength(Number(event.target.value) || DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH)
            }
            disabled={isSaving}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Last {{min}}-{{max}} alphanumeric characters of each key are appended to the channel name.',
              {
                min: MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
                max: MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
              }
            )}
          </p>
        </div>
      </div>
    </Dialog>
  )
}

function trimmedPreviewName(name: string): string {
  const trimmed = name.trim()
  return trimmed || 'template'
}
