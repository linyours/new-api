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

function isJsonObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function vertexAccountList(parsed: unknown): string[] {
  if (Array.isArray(parsed)) {
    if (parsed.length === 0) return []
    const accounts: string[] = []
    for (const item of parsed) {
      if (!isJsonObject(item)) return []
      accounts.push(JSON.stringify(item))
    }
    return accounts
  }
  if (!isJsonObject(parsed)) return []
  return [JSON.stringify(parsed)]
}

/**
 * Parse a Vertex service-account file or paste.
 * Accepts one JSON object, or a JSON array of objects.
 * Invalid JSON throws. Any other JSON shape returns an empty list.
 */
export function parseVertexServiceAccountText(raw: string): string[] {
  const trimmed = raw.trim()
  if (!trimmed) return []
  return vertexAccountList(JSON.parse(trimmed) as unknown)
}

/** Rebuild the JSON array payload expected by template apply. */
export function serializeVertexTemplateKeys(keys: string[]): string {
  return JSON.stringify(
    keys.map((key) => {
      try {
        return JSON.parse(key) as unknown
      } catch {
        return key
      }
    })
  )
}

/**
 * Replace one row with the first account and insert the rest after it.
 * The original row keeps its proxy; inserted rows start with an empty proxy.
 */
export function insertVertexAccountsAtRow<T extends { key: string }>(
  rows: T[],
  index: number,
  accounts: string[],
  createRow: (account: string) => T
): T[] {
  if (accounts.length === 0 || index < 0 || index >= rows.length) return rows
  const first = { ...rows[index], key: accounts[0] }
  const extras = accounts.slice(1).map((account) => createRow(account))
  return [...rows.slice(0, index), first, ...extras, ...rows.slice(index + 1)]
}

export type VertexTemplateTextRow = {
  key: string
  draft: string
  proxy: string
  jsonError: boolean
}

/**
 * Store pasted Vertex JSON on one row.
 * One object stays on that row. An array is split into following rows and the
 * pasted text is cleared so it cannot be submitted twice.
 */
export function applyVertexTextToRow<T extends VertexTemplateTextRow>(
  rows: T[],
  index: number,
  raw: string,
  createRow: (account: string) => T
): T[] {
  if (index < 0 || index >= rows.length) return rows
  const current = rows[index]
  const trimmed = raw.trim()
  if (!trimmed) {
    const next = [...rows]
    next[index] = { ...current, draft: raw, key: '', jsonError: false }
    return next
  }

  let accounts: string[] = []
  let invalid = false
  try {
    accounts = parseVertexServiceAccountText(trimmed)
    invalid = accounts.length === 0
  } catch {
    invalid = true
  }
  if (invalid) {
    const next = [...rows]
    next[index] = { ...current, draft: raw, key: '', jsonError: true }
    return next
  }

  if (accounts.length === 1) {
    const pretty = JSON.stringify(JSON.parse(accounts[0]), null, 2)
    const next = [...rows]
    next[index] = {
      ...current,
      draft: raw.includes('\n') ? raw : pretty,
      key: accounts[0],
      jsonError: false,
    }
    return next
  }

  const inserted = insertVertexAccountsAtRow(rows, index, accounts, createRow)
  inserted[index] = { ...inserted[index], draft: '', jsonError: false }
  return inserted
}

/**
 * Resolve rows into submit items. Non-empty text is parsed again and must be
 * valid JSON. A row with invalid text fails the whole result.
 */
export function validateVertexTemplateRows<
  T extends { key: string; draft: string; proxy: string },
>(rows: T[]): { items: ChannelTemplateApplyItem[]; invalid: boolean } {
  const items: ChannelTemplateApplyItem[] = []
  let invalid = false
  for (const row of rows) {
    const draft = row.draft.trim()
    if (draft) {
      try {
        const accounts = parseVertexServiceAccountText(draft)
        if (accounts.length === 0) {
          invalid = true
          continue
        }
        accounts.forEach((account, accountIndex) => {
          items.push({
            key: account,
            proxy: accountIndex === 0 ? row.proxy.trim() : '',
          })
        })
      } catch {
        invalid = true
      }
      continue
    }

    const key = row.key.trim()
    if (!key) continue
    try {
      const accounts = parseVertexServiceAccountText(key)
      if (accounts.length !== 1) {
        invalid = true
        continue
      }
      items.push({ key: accounts[0], proxy: row.proxy.trim() })
    } catch {
      invalid = true
    }
  }
  if (invalid) return { items: [], invalid: true }
  return { items, invalid: false }
}

/** Append newly parsed Vertex accounts without duplicating the same identity. */
export function mergeVertexTemplateKeys(
  existingRaw: string,
  incoming: string[]
): string[] {
  const existing = splitTemplateApplyKeys(existingRaw, { vertexJson: true })
  const seen = new Set<string>()
  const merged: string[] = []
  for (const key of [...existing, ...incoming]) {
    const id = channelTemplateKeyIdentity(key) || key
    if (seen.has(id)) continue
    seen.add(id)
    merged.push(key)
  }
  return merged
}

export function splitTemplateApplyKeys(
  raw: string,
  options?: { vertexJson?: boolean }
): string[] {
  const trimmed = raw.trim()
  if (!trimmed) return []
  if (
    options?.vertexJson &&
    (trimmed.startsWith('[') || trimmed.startsWith('{'))
  ) {
    try {
      return parseVertexServiceAccountText(trimmed)
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
