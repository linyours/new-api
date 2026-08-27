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
import assert from 'node:assert/strict'
import { beforeEach, describe, test } from 'node:test'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
} from '../channel-form'
import {
  channelKeyQuotaUnitsToUSD,
  channelKeyUSDToQuotaUnits,
  isValidChannelKeyUSDLimit,
  MAX_CHANNEL_KEY_QUOTA_UNITS,
} from '../key-quota-usd'

beforeEach(() => {
  useSystemConfigStore.setState((state) => ({
    config: {
      ...state.config,
      currency: { ...DEFAULT_CURRENCY_CONFIG },
    },
  }))
})

describe('channel key USD quota conversion', () => {
  test('converts API quota units to USD and back without changing storage units', () => {
    assert.equal(channelKeyQuotaUnitsToUSD(100_000_000), 200)
    assert.equal(channelKeyUSDToQuotaUnits(200), 100_000_000)
  })

  test('converts a channel form USD limit to API quota units', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'USD-limited channel',
      key: 'test-key',
      models: 'gpt-4o',
      key_quota_limit: 200,
    })

    assert.equal(payload.channel.key_quota_limit, 100_000_000)
  })

  test('preserves small fractional USD limits', () => {
    assert.equal(channelKeyQuotaUnitsToUSD(200), 0.0004)
    assert.equal(channelKeyUSDToQuotaUnits(0.0004), 200)
  })

  test('rejects USD values that exceed the safe quota-unit limit', () => {
    const maximumUSD =
      MAX_CHANNEL_KEY_QUOTA_UNITS / DEFAULT_CURRENCY_CONFIG.quotaPerUnit

    assert.equal(isValidChannelKeyUSDLimit(maximumUSD), true)
    assert.equal(isValidChannelKeyUSDLimit(maximumUSD + 0.01), false)
  })
})
