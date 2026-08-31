/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import { applyChannelTemplate, getChannelTemplates } from '../../api'
import { SUCCESS_MESSAGES } from '../../constants'
import {
  DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  buildChannelTemplateChannelName,
  channelsQueryKeys,
  getChannelTypeLabel,
  parseChannelTemplateConfig,
  splitTemplateApplyKeys,
} from '../../lib'
import { useChannels } from '../channels-provider'

type ApplyChannelTemplateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApplyChannelTemplateDialog({
  open,
  onOpenChange,
}: ApplyChannelTemplateDialogProps) {
  const { t } = useTranslation()
  const { applyTemplateId, setApplyTemplateId } = useChannels()
  const queryClient = useQueryClient()
  const [selectedId, setSelectedId] = useState<string>('')
  const [keys, setKeys] = useState('')
  const [suffixLength, setSuffixLength] = useState(
    DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  )
  const [isApplying, setIsApplying] = useState(false)

  const templatesQuery = useQuery({
    queryKey: channelsQueryKeys.templates(),
    queryFn: getChannelTemplates,
    enabled: open,
  })
  const templates = templatesQuery.data?.data ?? []

  useEffect(() => {
    if (!open) return
    if (applyTemplateId) {
      setSelectedId(String(applyTemplateId))
      return
    }
    if (templatesQuery.data?.data?.length === 1) {
      setSelectedId(String(templatesQuery.data.data[0].id))
    }
  }, [open, applyTemplateId, templatesQuery.data])

  useEffect(() => {
    if (!open) {
      setKeys('')
      setApplyTemplateId(null)
    }
  }, [open, setApplyTemplateId])

  const selectedTemplate = templates.find(
    (template) => String(template.id) === selectedId
  )

  useEffect(() => {
    if (!selectedTemplate) return
    setSuffixLength(
      selectedTemplate.name_suffix_length ||
        DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
    )
  }, [selectedTemplate])

  const selectedConfig = selectedTemplate
    ? parseChannelTemplateConfig(
        selectedTemplate.config,
        selectedTemplate.channel_type
      )
    : null

  const previewNames = useMemo(() => {
    if (!selectedTemplate) return []
    const parsedKeys = splitTemplateApplyKeys(keys, {
      vertexJson: selectedConfig?.vertexJson,
    })
    return parsedKeys.slice(0, 8).map((key) =>
      buildChannelTemplateChannelName(
        selectedTemplate.name,
        key,
        suffixLength
      )
    )
  }, [keys, selectedConfig?.vertexJson, selectedTemplate, suffixLength])

  const keyCount = splitTemplateApplyKeys(keys, {
    vertexJson: selectedConfig?.vertexJson,
  }).length

  const handleApply = async () => {
    if (!selectedTemplate) {
      toast.error(t('Select a channel template'))
      return
    }
    if (keyCount === 0) {
      toast.error(t('Enter one key per line'))
      return
    }
    setIsApplying(true)
    try {
      const response = await applyChannelTemplate(selectedTemplate.id, {
        keys,
        name_suffix_length: suffixLength,
      })
      if (!response.success) {
        toast.error(
          response.message || t('Failed to create channels from template')
        )
        return
      }
      toast.success(
        t(SUCCESS_MESSAGES.TEMPLATE_APPLIED, {
          count: response.data?.count ?? keyCount,
        })
      )
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      onOpenChange(false)
    } catch {
      toast.error(t('Failed to create channels from template'))
    } finally {
      setIsApplying(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Create from Template')}
      description={t(
        'Paste keys to create one channel per key. Names use the template name plus the last characters of each key.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isApplying}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleApply}
            disabled={isApplying || templates.length === 0}
          >
            {isApplying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Create Channels')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-2'>
        {templates.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t(
              'No channel templates yet. Save a configured channel as a template first.'
            )}
          </p>
        ) : (
          <>
            <div className='space-y-2'>
              <Label>{t('Template')}</Label>
              <Select
                items={templates.map((template) => ({
                  value: String(template.id),
                  label: template.name,
                }))}
                value={selectedId}
                onValueChange={(value) => {
                  if (value) setSelectedId(value)
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder={t('Select a channel template')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {templates.map((template) => (
                      <SelectItem
                        key={template.id}
                        value={String(template.id)}
                      >
                        {template.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {selectedTemplate ? (
                <p className='text-muted-foreground text-xs'>
                  {t(getChannelTypeLabel(selectedTemplate.channel_type))}
                  {selectedConfig?.models
                    ? ` · ${selectedConfig.models.split(',').slice(0, 4).join(', ')}`
                    : ''}
                </p>
              ) : null}
            </div>
            <div className='space-y-2'>
              <Label htmlFor='template-keys'>{t('API Keys')}</Label>
              <Textarea
                id='template-keys'
                value={keys}
                onChange={(event) => setKeys(event.target.value)}
                placeholder={
                  selectedConfig?.vertexJson
                    ? t('Paste a JSON array of Vertex service accounts')
                    : t('Enter one key per line')
                }
                rows={8}
                disabled={isApplying}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='apply-suffix-length'>
                {t('Key suffix length')}
              </Label>
              <Input
                id='apply-suffix-length'
                type='number'
                min={4}
                max={16}
                value={suffixLength}
                onChange={(event) =>
                  setSuffixLength(
                    Number(event.target.value) ||
                      DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
                  )
                }
                disabled={isApplying}
              />
            </div>
            {previewNames.length > 0 ? (
              <div className='space-y-1'>
                <p className='text-sm font-medium'>
                  {t('Name preview ({{count}} keys)', { count: keyCount })}
                </p>
                <ul className='text-muted-foreground text-xs'>
                  {previewNames.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                  {keyCount > previewNames.length ? (
                    <li>{t('and {{count}} more', { count: keyCount - previewNames.length })}</li>
                  ) : null}
                </ul>
              </div>
            ) : null}
          </>
        )}
      </div>
    </Dialog>
  )
}
