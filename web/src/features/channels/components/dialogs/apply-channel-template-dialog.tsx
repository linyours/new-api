/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
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
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../../constants'
import {
  DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  buildChannelTemplateChannelName,
  channelsQueryKeys,
  getChannelTypeLabel,
  isOptionalProxyURL,
  normalizeTemplateApplyItems,
  parseChannelTemplateConfig,
  parseTemplateApplyKeyProxyLines,
  splitTemplateApplyKeys,
  type ChannelTemplateApplyItem,
} from '../../lib'
import { useChannels } from '../channels-provider'

type ApplyChannelTemplateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ApplyRow = ChannelTemplateApplyItem & { id: number }

function createEmptyRow(id: number): ApplyRow {
  return { id, key: '', proxy: '' }
}

export function ApplyChannelTemplateDialog({
  open,
  onOpenChange,
}: ApplyChannelTemplateDialogProps) {
  const { t } = useTranslation()
  const { applyTemplateId, setApplyTemplateId } = useChannels()
  const queryClient = useQueryClient()
  const nextRowId = useRef(1)
  const [selectedId, setSelectedId] = useState<string>('')
  const [vertexKeys, setVertexKeys] = useState('')
  const [rows, setRows] = useState<ApplyRow[]>([createEmptyRow(0)])
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
      setVertexKeys('')
      setRows([createEmptyRow(0)])
      nextRowId.current = 1
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
  const isVertexJson = Boolean(selectedConfig?.vertexJson)

  const applyItems = useMemo(() => {
    if (isVertexJson) {
      return splitTemplateApplyKeys(vertexKeys, { vertexJson: true }).map(
        (key) => ({ key, proxy: '' })
      )
    }
    return normalizeTemplateApplyItems(rows)
  }, [isVertexJson, rows, vertexKeys])

  const previewNames = useMemo(() => {
    if (!selectedTemplate) return []
    return applyItems
      .slice(0, 8)
      .map((item) =>
        buildChannelTemplateChannelName(
          selectedTemplate.name,
          item.key,
          suffixLength
        )
      )
  }, [applyItems, selectedTemplate, suffixLength])

  const keyCount = applyItems.length

  const updateRow = (
    id: number,
    field: 'key' | 'proxy',
    value: string
  ) => {
    setRows((prev) =>
      prev.map((row) => (row.id === id ? { ...row, [field]: value } : row))
    )
  }

  const addRow = () => {
    const id = nextRowId.current++
    setRows((prev) => [...prev, createEmptyRow(id)])
  }

  const removeRow = (id: number) => {
    setRows((prev) => {
      if (prev.length <= 1) {
        return [createEmptyRow(nextRowId.current++)]
      }
      return prev.filter((row) => row.id !== id)
    })
  }

  const handlePasteKeys = (raw: string) => {
    const parsed = parseTemplateApplyKeyProxyLines(raw)
    if (parsed.length === 0) return
    setRows(
      parsed.map((item) => ({
        id: nextRowId.current++,
        key: item.key,
        proxy: item.proxy,
      }))
    )
  }

  const handleApply = async () => {
    if (!selectedTemplate) {
      toast.error(t('Select a channel template'))
      return
    }
    if (keyCount === 0) {
      toast.error(
        isVertexJson
          ? t('Enter one key per line')
          : t('Enter at least one API key')
      )
      return
    }
    if (!isVertexJson) {
      for (const item of applyItems) {
        if (!isOptionalProxyURL(item.proxy)) {
          toast.error(t(ERROR_MESSAGES.INVALID_PROXY))
          return
        }
      }
    }

    setIsApplying(true)
    try {
      const response = await applyChannelTemplate(selectedTemplate.id, {
        ...(isVertexJson
          ? { keys: vertexKeys }
          : { items: applyItems.map((item) => ({ key: item.key, proxy: item.proxy })) }),
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
      description={
        isVertexJson
          ? t(
              'Paste keys to create one channel per key. Names use the template name plus the last characters of each key.'
            )
          : t(
              'Add one API key per row. Optionally set a SOCKS5/HTTP proxy for that channel; leave proxy empty for no proxy.'
            )
      }
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

            {isVertexJson ? (
              <div className='space-y-2'>
                <Label htmlFor='template-keys'>{t('API Keys')}</Label>
                <Textarea
                  id='template-keys'
                  value={vertexKeys}
                  onChange={(event) => setVertexKeys(event.target.value)}
                  placeholder={t(
                    'Paste a JSON array of Vertex service accounts'
                  )}
                  rows={8}
                  disabled={isApplying}
                />
              </div>
            ) : (
              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-2'>
                  <Label>{t('API Keys')}</Label>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={addRow}
                    disabled={isApplying}
                  >
                    <Plus className='mr-1 h-4 w-4' />
                    {t('Add row')}
                  </Button>
                </div>
                <div className='space-y-2'>
                  <div className='text-muted-foreground grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2 text-xs font-medium'>
                    <span>{t('API Key')}</span>
                    <span>{t('Proxy (optional)')}</span>
                    <span className='w-9' />
                  </div>
                  {rows.map((row) => (
                    <div
                      key={row.id}
                      className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2'
                    >
                      <Input
                        value={row.key}
                        onChange={(event) =>
                          updateRow(row.id, 'key', event.target.value)
                        }
                        onPaste={(event) => {
                          const text = event.clipboardData.getData('text')
                          if (!text.includes('\n') && !text.includes('\t')) {
                            return
                          }
                          event.preventDefault()
                          handlePasteKeys(text)
                        }}
                        placeholder={t('sk-...')}
                        disabled={isApplying}
                        autoComplete='off'
                      />
                      <Input
                        value={row.proxy}
                        onChange={(event) =>
                          updateRow(row.id, 'proxy', event.target.value)
                        }
                        placeholder={t('socks5://user:pass@host:port')}
                        disabled={isApplying}
                        autoComplete='off'
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        className='shrink-0'
                        onClick={() => removeRow(row.id)}
                        disabled={isApplying}
                        aria-label={t('Remove row')}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  ))}
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Paste multiple lines into the key field to fill rows. Use tab or a space before socks5://... / http(s)://... to include a proxy.'
                  )}
                </p>
              </div>
            )}

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
                    <li>
                      {t('and {{count}} more', {
                        count: keyCount - previewNames.length,
                      })}
                    </li>
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
