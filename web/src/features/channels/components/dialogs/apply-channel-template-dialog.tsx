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

import {
  applyChannelTemplate,
  getChannelTemplates,
  standbyPoolQueryKey,
} from '../../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../../constants'
import {
  DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH,
  applyVertexTextToRow,
  buildChannelTemplateChannelName,
  channelTemplateKeyIdentity,
  channelsQueryKeys,
  getChannelTypeLabel,
  insertVertexAccountsAtRow,
  isOptionalProxyURL,
  normalizeTemplateApplyItems,
  parseChannelTemplateConfig,
  parseTemplateApplyKeyProxyLines,
  parseVertexServiceAccountText,
  validateVertexTemplateRows,
  type ChannelTemplateApplyItem,
} from '../../lib'
import { useOptionalChannels } from '../channels-provider'

type ApplyChannelTemplateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode?: 'channel' | 'standby'
  presetTemplateId?: number | null
}

type ApplyRow = ChannelTemplateApplyItem & {
  id: number
  draft: string
  jsonError: boolean
}

function createEmptyRow(id: number): ApplyRow {
  return { id, key: '', draft: '', proxy: '', jsonError: false }
}

export function ApplyChannelTemplateDialog({
  open,
  onOpenChange,
  mode = 'channel',
  presetTemplateId = null,
}: ApplyChannelTemplateDialogProps) {
  const { t } = useTranslation()
  const channels = useOptionalChannels()
  const applyTemplateId = presetTemplateId ?? channels?.applyTemplateId ?? null
  const queryClient = useQueryClient()
  const nextRowId = useRef(1)
  const [selectedId, setSelectedId] = useState<string>('')
  const [rows, setRows] = useState<ApplyRow[]>([createEmptyRow(0)])
  const [suffixLength, setSuffixLength] = useState(
    DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  )
  const [isApplying, setIsApplying] = useState(false)
  const destination = mode === 'standby' ? 'standby' : 'enabled'

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
      setRows([createEmptyRow(0)])
      nextRowId.current = 1
      setSelectedId('')
      channels?.setApplyTemplateId(null)
    }
  }, [open, channels])

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

  const vertexResolved = useMemo(
    () => (isVertexJson ? validateVertexTemplateRows(rows) : null),
    [isVertexJson, rows]
  )

  const applyItems = useMemo(() => {
    if (vertexResolved) return vertexResolved.items
    return normalizeTemplateApplyItems(rows)
  }, [rows, vertexResolved])

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

  const updateRow = (id: number, field: 'key' | 'proxy', value: string) => {
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
        draft: '',
        proxy: item.proxy,
        jsonError: false,
      }))
    )
  }

  const commitVertexAccounts = (rowId: number, accounts: string[]) => {
    setRows((prev) => {
      const index = prev.findIndex((row) => row.id === rowId)
      if (index < 0 || accounts.length === 0) return prev
      const next = insertVertexAccountsAtRow(
        prev,
        index,
        accounts,
        (account) => ({
          id: nextRowId.current++,
          key: account,
          draft: '',
          proxy: '',
          jsonError: false,
        })
      )
      next[index] = { ...next[index], draft: '', jsonError: false }
      return next
    })
  }

  const handleVertexRowFile = async (
    rowId: number,
    file: File,
    input: HTMLInputElement
  ) => {
    let accounts: string[]
    try {
      accounts = parseVertexServiceAccountText(await file.text())
    } catch {
      input.value = ''
      toast.error(t('Failed to parse JSON file: {{name}}', { name: file.name }))
      return
    }
    if (accounts.length === 0) {
      input.value = ''
      toast.error(t('Failed to parse JSON file: {{name}}', { name: file.name }))
      return
    }

    commitVertexAccounts(rowId, accounts)
    if (accounts.length > 1) {
      toast.success(
        t('{{count}} service account(s) loaded', { count: accounts.length })
      )
    }
  }

  const handleVertexDraft = (rowId: number, raw: string) => {
    let accountCount = 0
    const trimmed = raw.trim()
    if (trimmed) {
      try {
        accountCount = parseVertexServiceAccountText(trimmed).length
      } catch {
        accountCount = 0
      }
    }
    setRows((prev) => {
      const index = prev.findIndex((row) => row.id === rowId)
      if (index < 0) return prev
      return applyVertexTextToRow(prev, index, raw, (account) => ({
        id: nextRowId.current++,
        key: account,
        draft: '',
        proxy: '',
        jsonError: false,
      }))
    })
    if (accountCount > 1) {
      toast.success(
        t('{{count}} service account(s) loaded', { count: accountCount })
      )
    }
  }

  const handleApply = async () => {
    if (!selectedTemplate) {
      toast.error(t('Select a channel template'))
      return
    }
    if (isVertexJson && vertexResolved?.invalid) {
      toast.error(t('Invalid JSON'))
      return
    }
    if (keyCount === 0) {
      toast.error(
        isVertexJson
          ? t(
              'Add a service account JSON file or paste JSON for at least one row'
            )
          : t('Enter at least one API key')
      )
      return
    }
    for (const item of applyItems) {
      if (!isOptionalProxyURL(item.proxy)) {
        toast.error(t(ERROR_MESSAGES.INVALID_PROXY))
        return
      }
    }

    setIsApplying(true)
    try {
      const response = await applyChannelTemplate(selectedTemplate.id, {
        items: applyItems.map((item) => ({
          key: item.key,
          proxy: item.proxy,
        })),
        name_suffix_length: suffixLength,
        destination,
      })
      if (!response.success) {
        toast.error(
          response.message || t('Failed to create channels from template')
        )
        return
      }
      const createdCount = response.data?.count ?? keyCount
      const promotedCount = response.data?.promoted ?? 0
      if (promotedCount > 0) {
        toast.success(
          t(
            'Created {{count}} channels and promoted {{promoted}} into service',
            { count: createdCount, promoted: promotedCount }
          )
        )
      } else if (destination === 'standby') {
        toast.success(
          t('Added {{count}} channels to the standby pool', {
            count: createdCount,
          })
        )
      } else {
        toast.success(
          t(SUCCESS_MESSAGES.TEMPLATE_APPLIED, {
            count: createdCount,
          })
        )
      }
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.templates(),
      })
      queryClient.invalidateQueries({ queryKey: standbyPoolQueryKey })
      onOpenChange(false)
    } catch {
      toast.error(t('Failed to create channels from template'))
    } finally {
      setIsApplying(false)
    }
  }

  let dialogDescription = t(
    'Add one API key per row. Optionally set a SOCKS5/HTTP proxy for that channel; leave proxy empty for no proxy.'
  )
  if (isVertexJson) {
    dialogDescription = t(
      'Add one service account per row by uploading a JSON file or pasting JSON. Optionally set a SOCKS5/HTTP proxy for that channel; leave proxy empty for no proxy.'
    )
  }
  if (mode === 'standby' && isVertexJson) {
    dialogDescription = t(
      'Choose a template and add one service account per row. Each account enters this standby pool.'
    )
  } else if (mode === 'standby') {
    dialogDescription = t(
      'Choose a template and add one key per row. Each key enters this standby pool.'
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        mode === 'standby'
          ? t('Import into standby pool')
          : t('Create from Template')
      }
      description={dialogDescription}
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
            {destination === 'standby'
              ? t('Add to standby pool')
              : t('Create Channels')}
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
                      <SelectItem key={template.id} value={String(template.id)}>
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
              <div className='flex items-center justify-between gap-2'>
                <Label>
                  {isVertexJson
                    ? t('Service account JSON file(s)')
                    : t('API Keys')}
                </Label>
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
                  <span>
                    {isVertexJson ? t('JSON file or text') : t('API Key')}
                  </span>
                  <span>{t('Proxy (optional)')}</span>
                  <span className='w-9' />
                </div>
                {rows.map((row) =>
                  isVertexJson ? (
                    <div key={row.id} className='space-y-2'>
                      <div className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-2'>
                        <Input
                          type='file'
                          accept='.json,application/json'
                          disabled={isApplying}
                          aria-label={t('Service account JSON file(s)')}
                          onChange={(event) => {
                            const input = event.currentTarget
                            const file = input.files?.[0]
                            if (!file) return
                            void handleVertexRowFile(row.id, file, input)
                          }}
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
                      <Textarea
                        value={row.draft}
                        onChange={(event) =>
                          handleVertexDraft(row.id, event.target.value)
                        }
                        placeholder={t('Paste a service account JSON object')}
                        rows={6}
                        disabled={isApplying}
                        aria-invalid={row.jsonError || undefined}
                        autoComplete='off'
                        spellCheck={false}
                        className='field-sizing-fixed max-h-80 overflow-auto font-mono text-xs whitespace-pre'
                      />
                      {row.jsonError ? (
                        <p className='text-destructive text-xs'>
                          {t('Invalid JSON')}
                        </p>
                      ) : null}
                      {row.key ? (
                        <p className='text-muted-foreground truncate text-xs'>
                          {channelTemplateKeyIdentity(row.key)}
                        </p>
                      ) : null}
                    </div>
                  ) : (
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
                  )
                )}
              </div>
              <p className='text-muted-foreground text-xs'>
                {isVertexJson
                  ? t(
                      'Upload a JSON file or paste JSON in each row. The text must be a JSON object or an array of objects.'
                    )
                  : t(
                      'Paste multiple lines into the key field to fill rows. Use tab or a space before socks5://... / http(s)://... to include a proxy.'
                    )}
              </p>
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
