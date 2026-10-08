/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  parsePastedModelRpmLimits,
  parsePastedModels,
} from '../channel-batch-models'

describe('parsePastedModels', () => {
  it('splits comma and newline lists, trims names, and drops duplicates', () => {
    assert.deepEqual(
      parsePastedModels(' gpt-4o , gpt-4o-mini\ngpt-5\ngpt-4o，gpt-5'),
      ['gpt-4o', 'gpt-4o-mini', 'gpt-5']
    )
  })

  it('returns an empty list for blank input', () => {
    assert.deepEqual(parsePastedModels('  \n , '), [])
  })
})

describe('parsePastedModelRpmLimits', () => {
  it('accepts colon, comma, and whitespace separators', () => {
    assert.deepEqual(
      parsePastedModelRpmLimits('gpt-4o:20\ngpt-4o-mini, 0\nclaude-3 8'),
      {
        'gpt-4o': 20,
        'gpt-4o-mini': 0,
        'claude-3': 8,
      }
    )
  })

  it('rejects blank input, invalid RPM values, and duplicate models', () => {
    assert.equal(parsePastedModelRpmLimits(''), null)
    assert.equal(parsePastedModelRpmLimits('gpt-4o:-1'), null)
    assert.equal(parsePastedModelRpmLimits('gpt-4o:1.5'), null)
    assert.equal(parsePastedModelRpmLimits('gpt-4o:10\ngpt-4o:20'), null)
    assert.equal(parsePastedModelRpmLimits('gpt-4o'), null)
  })
})
