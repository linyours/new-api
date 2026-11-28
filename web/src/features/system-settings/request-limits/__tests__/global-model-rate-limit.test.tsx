/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { GlobalModelRateLimitSection } from '../global-model-rate-limit-section'
import { parseGlobalModelRPMLimits } from '../lib/global-model-rate-limit'

function SettingsFixture(props: { value: string }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <GlobalModelRateLimitSection value={props.value} />
    </SettingsPageProvider>
  )
}

function renderSettings(value = '{"enabled":false,"limits":{}}') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SettingsFixture value={value} />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

describe('site-wide model RPM settings', () => {
  it('saves the enable flag and normalized model limits in one request', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const user = renderSettings()
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'Add Row' }))
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    expect(
      screen.getByRole('group', { name: 'Model RPM limits' })
    ).toHaveAttribute('aria-invalid', 'true')
    await user.type(
      screen.getByRole('textbox', { name: 'Model name' }),
      ' gpt-4o '
    )
    await user.type(screen.getByRole('textbox', { name: 'RPM limit' }), '300')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
    )
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'GlobalModelRateLimit',
        value: '{"enabled":true,"limits":{"gpt-4o":300}}',
      })
    )
    expect(put).toHaveBeenCalledTimes(1)
  })

  it('blocks duplicate and invalid rows until corrected or deleted', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const user = renderSettings('{"enabled":true,"limits":{"gpt-4o":3}}')
    await user.click(screen.getByRole('button', { name: 'Add Row' }))
    await user.type(
      screen.getAllByRole('textbox', { name: 'Model name' })[1],
      'gpt-4o'
    )
    await user.type(
      screen.getAllByRole('textbox', { name: 'RPM limit' })[1],
      '4'
    )
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    expect(put).not.toHaveBeenCalled()
    await user.click(screen.getAllByRole('button', { name: 'Delete row' })[1])
    const rpm = screen.getByRole('textbox', { name: 'RPM limit' })
    await user.clear(rpm)
    await user.type(rpm, '-1')
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    await user.clear(rpm)
    await user.type(rpm, '0')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
    )
    expect(screen.getByRole('textbox', { name: 'Model name' })).toHaveValue(
      'gpt-4o'
    )
  })

  it('disables editing while saving and keeps the draft after a failed save', async () => {
    let rejectSave: (error: Error) => void = () => undefined
    vi.spyOn(api, 'put').mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectSave = reject
        })
    )
    const user = renderSettings('{"enabled":true,"limits":{"gpt-4o":3}}')
    const rpm = screen.getByRole('textbox', { name: 'RPM limit' })
    await user.clear(rpm)
    await user.type(rpm, '7')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
    )
    expect(rpm).toBeDisabled()
    expect(screen.getByRole('switch')).toHaveAttribute('aria-disabled', 'true')
    await act(async () => {
      rejectSave(new Error('Save failed'))
    })
    await waitFor(() => expect(rpm).toBeEnabled())
    expect(rpm).toHaveValue('7')
  })

  it('keeps unsaved model edits when the interface language changes', async () => {
    const user = renderSettings('{"enabled":true,"limits":{"gpt-4o":3}}')
    const rpm = screen.getByRole('textbox', { name: 'RPM limit' })
    await user.clear(rpm)
    await user.type(rpm, '9')
    try {
      await act(async () => {
        await i18next.changeLanguage('zh')
      })
      expect(rpm).toHaveValue('9')
    } finally {
      await act(async () => {
        await i18next.changeLanguage('en')
      })
    }
  })

  it('treats prototype-like model names as ordinary model names', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const user = renderSettings('{"enabled":true,"limits":{}}')
    await user.click(screen.getByRole('button', { name: 'Add Row' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Model name' }),
      '__proto__'
    )
    await user.type(screen.getByRole('textbox', { name: 'RPM limit' }), '1')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'GlobalModelRateLimit',
        value: '{"enabled":true,"limits":{"__proto__":1}}',
      })
    )
  })
})

describe('model RPM validation', () => {
  it.each([
    'null',
    '[]',
    '{"m":null}',
    '{"m":-1}',
    '{"m":1.5}',
    '{"m":100001}',
    '{"m":"1"}',
    '{" ":1}',
    '{"m":1," m ":2}',
  ])('rejects invalid configuration %s', (value) => {
    expect(parseGlobalModelRPMLimits(value)).toBeNull()
  })
  it('allows empty limits and zero while bounding model names and model count', () => {
    expect(parseGlobalModelRPMLimits('')).toEqual({})
    expect(parseGlobalModelRPMLimits('{" m ":0}')).toEqual({ m: 0 })
    expect(
      parseGlobalModelRPMLimits(JSON.stringify({ ['x'.repeat(256)]: 1 }))
    ).toBeNull()
    expect(
      parseGlobalModelRPMLimits(
        JSON.stringify(
          Object.fromEntries(
            Array.from({ length: 1001 }, (_, i) => [`model-${i}`, 1])
          )
        )
      )
    ).toBeNull()
  })
})
