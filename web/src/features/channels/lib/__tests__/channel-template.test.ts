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
  buildChannelTemplateChannelName,
  channelTemplateNameSuffix,
  normalizeTemplateApplyItems,
  parseTemplateApplyKeyProxyLines,
  insertVertexAccountsAtRow,
  mergeVertexTemplateKeys,
  parseVertexServiceAccountText,
  serializeVertexTemplateKeys,
  splitTemplateApplyKeys,
  validateVertexTemplateRows,
  applyVertexTextToRow,
} from '../channel-template'

describe('channel template naming', () => {
  it('uses the alphanumeric tail of a key', () => {
    assert.equal(channelTemplateNameSuffix('sk-proj-xxxxabcdef', 6), 'abcdef')
    assert.equal(channelTemplateNameSuffix('short1234', 6), 'rt1234')
    assert.equal(
      buildChannelTemplateChannelName('openai-prod', 'sk-xxxxabcdef', 6),
      'openai-prod-abcdef'
    )
  })

  it('uses credential identity for JSON and pipe-separated keys', () => {
    assert.equal(
      channelTemplateNameSuffix(
        '{"client_email":"svc-abcdef@project.iam.gserviceaccount.com"}',
        6
      ),
      'abcdef'
    )
    assert.equal(
      channelTemplateNameSuffix('AKIAKEYID12|secret|us-east-1', 7),
      'KEYID12'
    )
  })

  it('splits newline keys and Vertex JSON arrays', () => {
    assert.deepEqual(splitTemplateApplyKeys('sk-a\n\nsk-b\n'), ['sk-a', 'sk-b'])
    assert.deepEqual(
      splitTemplateApplyKeys('[{"client_email":"a@x.com"},{"client_email":"b@x.com"}]', {
        vertexJson: true,
      }),
      ['{"client_email":"a@x.com"}', '{"client_email":"b@x.com"}']
    )
  })

  it('accepts one Vertex service account object and rebuilds a JSON array', () => {
    const account = '{"client_email":"svc-abcdef@project.iam.gserviceaccount.com"}'
    assert.deepEqual(parseVertexServiceAccountText(account), [account])
    assert.deepEqual(
      splitTemplateApplyKeys(account, { vertexJson: true }),
      [account]
    )
    assert.equal(
      serializeVertexTemplateKeys([account]),
      '[{"client_email":"svc-abcdef@project.iam.gserviceaccount.com"}]'
    )
  })

  it('merges a Vertex JSON array file into one key per account', () => {
    const raw = '[{"client_email":"a@x.com"},{"client_email":"b@x.com"}]'
    assert.deepEqual(parseVertexServiceAccountText(raw), [
      '{"client_email":"a@x.com"}',
      '{"client_email":"b@x.com"}',
    ])
    assert.deepEqual(splitTemplateApplyKeys('{', { vertexJson: true }), [])
  })

  it('keeps the current row proxy when a JSON file contains several accounts', () => {
    const rows = [
      { id: 1, key: '', proxy: 'socks5://proxy.example:1080' },
      { id: 2, key: 'existing', proxy: '' },
    ]
    assert.deepEqual(
      insertVertexAccountsAtRow(
        rows,
        0,
        ['{"client_email":"a@x.com"}', '{"client_email":"b@x.com"}'],
        (account) => ({ id: 3, key: account, proxy: '' })
      ),
      [
        {
          id: 1,
          key: '{"client_email":"a@x.com"}',
          proxy: 'socks5://proxy.example:1080',
        },
        { id: 3, key: '{"client_email":"b@x.com"}', proxy: '' },
        { id: 2, key: 'existing', proxy: '' },
      ]
    )
  })

  it('rejects Vertex text that is not a JSON object', () => {
    assert.throws(() => parseVertexServiceAccountText('{'))
    assert.deepEqual(parseVertexServiceAccountText('123'), [])
    assert.deepEqual(parseVertexServiceAccountText('"account"'), [])
    assert.deepEqual(parseVertexServiceAccountText('[{"client_email":"a@x.com"}, "nope"]'), [])
  })

  it('keeps a pasted Vertex object on the same row and rejects invalid JSON', () => {
    const rows = [{ key: '', draft: '', proxy: 'socks5://proxy.example:1080', jsonError: false }]
    const valid = applyVertexTextToRow(
      rows,
      0,
      '{"client_email":"a@x.com"}',
      (account) => ({ key: account, draft: '', proxy: '', jsonError: false })
    )
    assert.equal(valid[0]?.jsonError, false)
    assert.equal(valid[0]?.key, '{"client_email":"a@x.com"}')
    assert.equal(valid[0]?.draft, '{\n  "client_email": "a@x.com"\n}')
    assert.equal(valid[0]?.proxy, 'socks5://proxy.example:1080')

    const formatted = '{\n  "client_email": "a@x.com"\n}'
    const kept = applyVertexTextToRow(
      rows,
      0,
      formatted,
      (account) => ({ key: account, draft: '', proxy: '', jsonError: false })
    )
    assert.equal(kept[0]?.draft, formatted)

    const invalid = applyVertexTextToRow(
      valid,
      0,
      '{bad',
      (account) => ({ key: account, draft: '', proxy: '', jsonError: false })
    )
    assert.equal(invalid[0]?.jsonError, true)
    assert.equal(invalid[0]?.key, '')
    assert.equal(
      validateVertexTemplateRows(invalid).invalid,
      true
    )
    assert.deepEqual(validateVertexTemplateRows(valid).items, [
      {
        key: '{"client_email":"a@x.com"}',
        proxy: 'socks5://proxy.example:1080',
      },
    ])
  })

  it('splits a pasted Vertex JSON array and does not submit the raw text again', () => {
    const rows = [{ key: '', draft: '', proxy: 'socks5://proxy.example:1080', jsonError: false }]
    const next = applyVertexTextToRow(
      rows,
      0,
      '[{"client_email":"a@x.com"},{"client_email":"b@x.com"}]',
      (account) => ({ key: account, draft: '', proxy: '', jsonError: false })
    )
    assert.equal(next[0]?.draft, '')
    assert.deepEqual(validateVertexTemplateRows(next), {
      invalid: false,
      items: [
        {
          key: '{"client_email":"a@x.com"}',
          proxy: 'socks5://proxy.example:1080',
        },
        { key: '{"client_email":"b@x.com"}', proxy: '' },
      ],
    })
  })

  it('submits an uploaded Vertex account and blocks invalid text on another row', () => {
    const uploaded = validateVertexTemplateRows([
      { key: '{"client_email":"a@x.com"}', draft: '', proxy: '' },
      { key: '', draft: '{bad', proxy: 'socks5://proxy.example:1080' },
    ])
    assert.equal(uploaded.invalid, true)
    assert.deepEqual(uploaded.items, [])
  })

  it('appends another Vertex account and skips a duplicate identity', () => {
    const first = '{"client_email":"a@x.com","project_id":"p1"}'
    const second = '{"client_email":"b@x.com","project_id":"p2"}'
    const duplicate = '{"project_id":"p1","client_email":"a@x.com"}'
    assert.deepEqual(
      mergeVertexTemplateKeys(`[${first}]`, [second, duplicate]),
      [first, second]
    )
  })

  it('parses key and optional proxy lines', () => {
    assert.deepEqual(
      parseTemplateApplyKeyProxyLines(
        'sk1\tsocks5://proxy1.example:1080\nsk2 socks5://user:pass@host:1080\nsk3\n'
      ),
      [
        { key: 'sk1', proxy: 'socks5://proxy1.example:1080' },
        { key: 'sk2', proxy: 'socks5://user:pass@host:1080' },
        { key: 'sk3', proxy: '' },
      ]
    )
    assert.deepEqual(
      normalizeTemplateApplyItems([
        { key: '  sk-a  ', proxy: '  ' },
        { key: '', proxy: 'socks5://x' },
        { key: 'sk-b', proxy: 'socks5://y' },
      ]),
      [
        { key: 'sk-a', proxy: '' },
        { key: 'sk-b', proxy: 'socks5://y' },
      ]
    )
  })
})
