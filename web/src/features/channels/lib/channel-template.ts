/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
export const DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH = 6
export const MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH = 4
export const MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH = 16

export function normalizeChannelTemplateNameSuffixLength(
  value: number | undefined
): number {
  if (!value || value <= 0) {
    return DEFAULT_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  }
  if (value < MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH) {
    return MIN_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  }
  if (value > MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH) {
    return MAX_CHANNEL_TEMPLATE_NAME_SUFFIX_LENGTH
  }
  return value
}

export function channelTemplateKeyIdentity(rawKey: string): string {
  const key = rawKey.trim()
  if (!key) return ''
  if (key.startsWith('{')) {
    try {
      const parsed = JSON.parse(key) as Record<string, unknown>
      for (const field of ['client_email', 'account_id', 'project_id']) {
        const value = String(parsed[field] ?? '').trim()
        if (!value || value === '<nil>') continue
        const atIndex = value.indexOf('@')
        if (atIndex > 0) {
          return value.slice(0, atIndex).trim()
        }
        return value
      }
    } catch {
      // Fall through to the raw key when JSON is incomplete.
    }
  }
  const pipeIndex = key.indexOf('|')
  if (pipeIndex > 0) {
    return key.slice(0, pipeIndex).trim()
  }
  return key
}

export function channelTemplateNameSuffix(
  rawKey: string,
  suffixLength?: number
): string {
  const length = normalizeChannelTemplateNameSuffixLength(suffixLength)
  const identity = channelTemplateKeyIdentity(rawKey)
  const alnum = [...identity]
    .filter((char) => /\p{L}|\p{N}/u.test(char))
    .join('')
  if (!alnum) return ''
  if (alnum.length <= length) return alnum
  return alnum.slice(-length)
}

export function buildChannelTemplateChannelName(
  baseName: string,
  rawKey: string,
  suffixLength?: number
): string {
  const trimmedName = baseName.trim()
  const suffix = channelTemplateNameSuffix(rawKey, suffixLength)
  if (!trimmedName) return suffix
  if (!suffix) return trimmedName
  return `${trimmedName}-${suffix}`
}

export type ChannelTemplateApplyItem = {
  key: string
  proxy: string
}

export function splitTemplateApplyKeys(
  raw: string,
  options?: { vertexJson?: boolean }
): string[] {
  const trimmed = raw.trim()
  if (!trimmed) return []
  if (options?.vertexJson && trimmed.startsWith('[')) {
    try {
      const parsed = JSON.parse(trimmed) as unknown
      if (!Array.isArray(parsed)) return []
      return parsed
        .map((item) => {
          if (typeof item === 'string') return item.trim()
          return JSON.stringify(item)
        })
        .filter(Boolean)
    } catch {
      return []
    }
  }
  return trimmed
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

/**
 * Parse pasted key[+proxy] lines for the apply-template form.
 * Supports: `key`, `key<TAB>proxy`, or `key socks5://...` / `key http(s)://...`.
 */
export function parseTemplateApplyKeyProxyLines(
  raw: string
): ChannelTemplateApplyItem[] {
  const items: ChannelTemplateApplyItem[] = []
  for (const line of raw.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue

    const tabIndex = trimmed.indexOf('\t')
    if (tabIndex >= 0) {
      const key = trimmed.slice(0, tabIndex).trim()
      const proxy = trimmed.slice(tabIndex + 1).trim()
      if (key) items.push({ key, proxy })
      continue
    }

    const proxyMatch = trimmed.match(
      /^(.+?)\s+((?:socks5h?|https?):\/\/\S+)\s*$/i
    )
    if (proxyMatch) {
      const key = proxyMatch[1].trim()
      const proxy = proxyMatch[2].trim()
      if (key) items.push({ key, proxy })
      continue
    }

    items.push({ key: trimmed, proxy: '' })
  }
  return items
}

export function normalizeTemplateApplyItems(
  items: ChannelTemplateApplyItem[]
): ChannelTemplateApplyItem[] {
  return items
    .map((item) => ({
      key: item.key.trim(),
      proxy: item.proxy.trim(),
    }))
    .filter((item) => item.key.length > 0)
}

export function parseChannelTemplateConfig(
  raw: string,
  channelType?: number
): {
  models: string
  group: string
  vertexJson: boolean
} {
  try {
    const parsed = JSON.parse(raw) as {
      models?: string
      group?: string
      settings?: string
    }
    let vertexKeyType = ''
    if (parsed.settings) {
      const settings = JSON.parse(parsed.settings) as {
        vertex_key_type?: string
      }
      vertexKeyType = settings.vertex_key_type || ''
    }
    return {
      models: parsed.models || '',
      group: parsed.group || 'default',
      vertexJson: channelType === 41 && vertexKeyType !== 'api_key',
    }
  } catch {
    return { models: '', group: 'default', vertexJson: false }
  }
}
