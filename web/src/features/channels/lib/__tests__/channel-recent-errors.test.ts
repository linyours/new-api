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
import { describe, test } from 'node:test'

import type { Channel } from '../../types'
import {
  canInspectChannelRecentErrors,
  channelHealthLayerLabel,
  recentErrorAccentClass,
  recentErrorStatusBadgeVariant,
  successRateBadgeVariant,
  successRateBarClass,
  successRateHeaderTint,
} from '../channel-recent-errors'
import type { TagRow } from '../channel-utils'

describe('channel recent errors helpers', () => {
  test('does not inspect tag aggregate rows', () => {
    const row = { id: 1, children: [{ id: 2 } as Channel] } as TagRow
    assert.equal(canInspectChannelRecentErrors(row), false)
  })

  test('does not inspect invalid channel ids', () => {
    assert.equal(canInspectChannelRecentErrors({ id: 0 } as Channel), false)
  })

  test('inspects real channel ids', () => {
    assert.equal(canInspectChannelRecentErrors({ id: 12 } as Channel), true)
  })

  test('maps success-rate badge variants from alert and warning thresholds', () => {
    assert.equal(successRateBadgeVariant(0.4, 0.5), 'danger')
    assert.equal(successRateBadgeVariant(0.7, 0.5), 'warning')
    assert.equal(successRateBadgeVariant(0.95, 0.5), 'success')
  })

  test('maps HTTP status badges by class', () => {
    assert.equal(recentErrorStatusBadgeVariant(502), 'destructive')
    assert.equal(recentErrorStatusBadgeVariant(429), 'warning')
    assert.equal(recentErrorStatusBadgeVariant(200), 'outline')
  })

  test('tints the popup header by success-rate severity', () => {
    assert.equal(
      successRateHeaderTint('danger'),
      'from-destructive/12 to-destructive/4'
    )
    assert.equal(
      successRateHeaderTint('warning'),
      'from-warning/12 to-warning/4'
    )
    assert.equal(
      successRateHeaderTint('success'),
      'from-success/12 to-success/4'
    )
  })

  test('colors the error accent bar by HTTP status class', () => {
    assert.equal(recentErrorAccentClass(502), 'bg-destructive')
    assert.equal(recentErrorAccentClass(429), 'bg-warning')
    assert.equal(recentErrorAccentClass(0), 'bg-border')
  })

  test('labels layered windows', () => {
    const t = (key: string, options?: { count?: number }) => {
      if (key === '{{count}} min') return `${options?.count} min`
      return key
    }
    assert.equal(channelHealthLayerLabel(1, t as never), '1 min')
    assert.equal(channelHealthLayerLabel(15, t as never), '15 min')
    assert.equal(channelHealthLayerLabel(60, t as never), '1 hour')
  })

  test('colors the layered success bar by severity', () => {
    assert.equal(successRateBarClass('danger'), 'bg-destructive')
    assert.equal(successRateBarClass('warning'), 'bg-warning')
    assert.equal(successRateBarClass('success'), 'bg-success')
  })
})
