/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { parseModelRpmLimits, unusedModelName } from '../model-rpm-limits'

describe('parseModelRpmLimits', () => {
  it('normalizes model names and preserves explicit unlimited limits', () => {
    assert.deepEqual(
      parseModelRpmLimits([
        { model: ' gpt-4o ', rpm: '10' },
        { model: 'gpt-4o-mini', rpm: '0' },
      ]),
      {
        'gpt-4o': 10,
        'gpt-4o-mini': 0,
      }
    )
  })

  it('rejects duplicate normalized model names', () => {
    assert.equal(
      parseModelRpmLimits([
        { model: 'gpt-4o', rpm: '10' },
        { model: ' gpt-4o ', rpm: '20' },
      ]),
      null
    )
  })

  it('rejects incomplete or invalid rows', () => {
    const invalidRows = [
      [{ model: '', rpm: '10' }],
      [{ model: 'gpt-4o', rpm: '' }],
      [{ model: 'gpt-4o', rpm: '-1' }],
      [{ model: 'gpt-4o', rpm: '1.5' }],
    ]
    for (const rows of invalidRows) {
      assert.equal(parseModelRpmLimits(rows), null)
    }
  })
})

describe('unusedModelName', () => {
  it('picks the first unused model name from the channel model list', () => {
    assert.equal(
      unusedModelName(['gpt-4o', 'gpt-4o-mini'], [{ model: 'gpt-4o' }]),
      'gpt-4o-mini'
    )
    assert.equal(unusedModelName(['gpt-4o'], [{ model: 'gpt-4o' }]), '')
  })
})
