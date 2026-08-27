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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import {
  channelKeyQuotaUnitsToUSD,
  channelKeyUSDToQuotaUnits,
  isValidChannelKeyUSDLimit,
} from '../../lib/key-quota-usd'
import { parseModelRpmLimits, unusedModelName } from '../../lib/model-rpm-limits'
import type { KeyStatus } from '../../types'
import {
  ModelRpmLimitsEditor,
  type ModelRpmLimitRow,
} from '../model-rpm-limits-editor'

type ChannelKeyLimitsDialogProps = {
  open: boolean
  keyStatus: KeyStatus | null
  saving: boolean
  models: string[]
  onOpenChange: (open: boolean) => void
  onSave: (
    rpmLimit: number | null,
    quotaLimit: number | null,
    modelRpmLimits: Record<string, number>
  ) => Promise<void>
}

function parseOptionalLimit(
  value: string,
  maximum: number
): number | null | undefined {
  if (value.trim() === '') return null
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < 0 || parsed > maximum) {
    return undefined
  }
  return parsed
}

function parseOptionalUSDLimit(value: string): number | null | undefined {
  if (value.trim() === '') return null
  const parsed = Number(value)
  if (!isValidChannelKeyUSDLimit(parsed)) return undefined
  return channelKeyUSDToQuotaUnits(parsed)
}

export function ChannelKeyLimitsDialog(props: ChannelKeyLimitsDialogProps) {
  const { t } = useTranslation()
  const [rpmLimit, setRpmLimit] = useState('')
  const [quotaLimit, setQuotaLimit] = useState('')
  const [modelRpmRows, setModelRpmRows] = useState<ModelRpmLimitRow[]>([])
  const [validationError, setValidationError] = useState('')
  const nextRowId = useRef(0)

  useEffect(() => {
    if (!props.open || !props.keyStatus) return
    setRpmLimit(
      props.keyStatus.rpm_limit == null ? '' : String(props.keyStatus.rpm_limit)
    )
    setQuotaLimit(
      props.keyStatus.quota_limit == null
        ? ''
        : String(channelKeyQuotaUnitsToUSD(props.keyStatus.quota_limit))
    )
    setModelRpmRows(
      Object.entries(props.keyStatus.model_rpm_limits || {})
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([model, rpm]) => ({
          id: nextRowId.current++,
          model,
          rpm: String(rpm),
        }))
    )
    setValidationError('')
  }, [props.keyStatus, props.open])

  const handleSave = async () => {
    const parsedRpm = parseOptionalLimit(rpmLimit, Number.MAX_SAFE_INTEGER)
    const parsedQuota = parseOptionalUSDLimit(quotaLimit)
    const modelRpmLimits = parseModelRpmLimits(modelRpmRows)
    if (
      parsedRpm === undefined ||
      parsedQuota === undefined ||
      modelRpmLimits === null
    ) {
      setValidationError(
        t(
          'RPM values must be non-negative integers, model names must be unique, and the USD limit must be a non-negative number.'
        )
      )
      return
    }
    await props.onSave(parsedRpm, parsedQuota, modelRpmLimits)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Configure key limits')}
      description={t(
        'Leave a field empty to inherit the channel default. Enter 0 for unlimited.'
      )}
      contentClassName='max-w-2xl'
    >
      <div className='space-y-5'>
        <div className='space-y-2'>
          <Label htmlFor='channel-key-rpm-limit'>
            {t('Per-key RPM limit')}
          </Label>
          <Input
            id='channel-key-rpm-limit'
            type='number'
            min={0}
            step={1}
            value={rpmLimit}
            onChange={(event) => setRpmLimit(event.target.value)}
            placeholder={t('Inherit channel default')}
          />
        </div>
        <ModelRpmLimitsEditor
          rows={modelRpmRows}
          models={props.models}
          disabled={props.saving}
          datalistId='channel-key-model-rpm-options'
          onChange={setModelRpmRows}
          onAdd={() => {
            const model = unusedModelName(props.models, modelRpmRows)
            setModelRpmRows((rows) => [
              ...rows,
              { id: nextRowId.current++, model, rpm: '' },
            ])
          }}
        />
        <div className='space-y-2'>
          <Label htmlFor='channel-key-quota-limit'>
            {t('Per-key USD limit')}
          </Label>
          <Input
            id='channel-key-quota-limit'
            type='number'
            min={0}
            step='any'
            value={quotaLimit}
            onChange={(event) => setQuotaLimit(event.target.value)}
            placeholder={t('Inherit channel default')}
          />
        </div>
        {validationError && (
          <p className='text-destructive text-sm' role='alert'>
            {validationError}
          </p>
        )}
        <div className='flex justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.saving}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSave} disabled={props.saving}>
            {t('Save changes')}
          </Button>
        </div>
      </div>
    </Dialog>
  )
}
