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
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import {
  handleBatchAddModelRpmLimits,
  parsePastedModelRpmLimits,
} from '../../lib'

type BatchAddModelRpmLimitsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  selectedIds: number[]
  onSuccess: () => void
}

export function BatchAddModelRpmLimitsDialog(
  props: BatchAddModelRpmLimitsDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rawLimits, setRawLimits] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const parsedLimits = useMemo(
    () => parsePastedModelRpmLimits(rawLimits),
    [rawLimits]
  )
  const hasInput = rawLimits.trim().length > 0
  const parsedCount = parsedLimits ? Object.keys(parsedLimits).length : 0
  let helperText = t(
    'These limits are merged onto every non-archived key. Enter 0 for no extra per-model cap.'
  )
  if (hasInput && parsedLimits) {
    helperText = t('{{count}} model RPM limit(s) will be applied', {
      count: parsedCount,
    })
  } else if (hasInput) {
    helperText = t(
      'Each line must be model and a non-negative integer RPM, with unique model names.'
    )
  }

  useEffect(() => {
    if (props.open) {
      setRawLimits('')
      setIsSaving(false)
    }
  }, [props.open])

  const handleClose = () => {
    if (isSaving) return
    props.onOpenChange(false)
  }

  const handleSubmit = async () => {
    if (isSaving || !parsedLimits) return
    setIsSaving(true)
    try {
      await handleBatchAddModelRpmLimits(
        props.selectedIds,
        parsedLimits,
        queryClient,
        () => {
          props.onOpenChange(false)
          props.onSuccess()
        }
      )
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) handleClose()
      }}
      title={t('Add model RPM limits')}
      description={
        <>
          {t('Add model RPM limits to')} {props.selectedIds.length}{' '}
          {t(
            'selected channel(s). Matching models overwrite existing RPM values on every non-archived key.'
          )}
        </>
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleClose} disabled={isSaving}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSaving || !parsedLimits}>
            {isSaving ? t('Saving...') : t('Add model RPM limits')}
          </Button>
        </>
      }
    >
      <div className='grid gap-2'>
        <Label htmlFor='batch-add-model-rpm-limits'>
          {t('Per-model RPM limits')}
        </Label>
        <Textarea
          id='batch-add-model-rpm-limits'
          value={rawLimits}
          onChange={(event) => setRawLimits(event.target.value)}
          disabled={isSaving}
          rows={6}
          placeholder={t('One per line, for example gpt-4o,60 or gpt-4o:60')}
        />
        <p className='text-muted-foreground text-xs'>{helperText}</p>
      </div>
    </Dialog>
  )
}
