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
  splitTemplateApplyKeys,
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
})
