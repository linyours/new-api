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

import type { ChannelHealth } from '../../types'
import { mergeChannelHealth } from '../channel-utils'

function layer(
  minutes: number,
  success: number,
  total: number
): NonNullable<ChannelHealth['layers']>[number] {
  return {
    minutes,
    window_seconds: minutes * 60,
    total,
    success,
    bad: total - success,
    success_rate: total > 0 ? success / total : 0,
    error_rate: total > 0 ? (total - success) / total : 0,
    min_total: 30,
    sample_ready: total >= 30,
  }
}

describe('mergeChannelHealth', () => {
  test('sums matching layered windows for tag rows', () => {
    const current: ChannelHealth = {
      window_seconds: 300,
      total: 10,
      success: 8,
      bad: 2,
      success_rate: 0.8,
      error_rate: 0.2,
      min_total: 30,
      alert_below: 0.5,
      sample_ready: false,
      layers: [layer(1, 1, 2), layer(5, 8, 10)],
    }
    const next: ChannelHealth = {
      window_seconds: 300,
      total: 20,
      success: 10,
      bad: 5,
      success_rate: 0.5,
      error_rate: 0.25,
      min_total: 30,
      alert_below: 0.5,
      sample_ready: false,
      layers: [layer(1, 3, 4), layer(5, 10, 20), layer(15, 12, 25)],
    }

    const merged = mergeChannelHealth(current, next)
    assert.equal(merged?.total, 30)
    assert.equal(merged?.success, 18)
    const byMinutes = new Map(
      (merged?.layers ?? []).map((item) => [item.minutes, item])
    )
    assert.equal(byMinutes.get(1)?.total, 6)
    assert.equal(byMinutes.get(1)?.success, 4)
    assert.equal(byMinutes.get(5)?.total, 30)
    assert.equal(byMinutes.get(5)?.success, 18)
    assert.equal(byMinutes.get(15)?.total, 25)
    assert.deepEqual(
      (merged?.layers ?? []).map((item) => item.minutes),
      [1, 5, 15]
    )
  })
})
