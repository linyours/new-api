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

import { handleBatchAddModels, parsePastedModels } from '../../lib'

type BatchAddModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  selectedIds: number[]
  onSuccess: () => void
}

export function BatchAddModelsDialog(props: BatchAddModelsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rawModels, setRawModels] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const parsedModels = useMemo(() => parsePastedModels(rawModels), [rawModels])

  useEffect(() => {
    if (props.open) {
      setRawModels('')
      setIsSaving(false)
    }
  }, [props.open])

  const handleClose = () => {
    if (isSaving) return
    props.onOpenChange(false)
  }

  const handleSubmit = async () => {
    if (isSaving || parsedModels.length === 0) return
    setIsSaving(true)
    try {
      await handleBatchAddModels(
        props.selectedIds,
        parsedModels,
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
      title={t('Add models')}
      description={
        <>
          {t('Add models to')} {props.selectedIds.length}{' '}
          {t('selected channel(s). Existing models are kept.')}
        </>
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleClose} disabled={isSaving}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={isSaving || parsedModels.length === 0}
          >
            {isSaving ? t('Saving...') : t('Add models')}
          </Button>
        </>
      }
    >
      <div className='grid gap-2'>
        <Label htmlFor='batch-add-models'>{t('Models')}</Label>
        <Textarea
          id='batch-add-models'
          value={rawModels}
          onChange={(event) => setRawModels(event.target.value)}
          disabled={isSaving}
          rows={6}
          placeholder={t('One model per line, or separate with commas')}
        />
        <p className='text-muted-foreground text-xs'>
          {parsedModels.length > 0
            ? t('{{count}} model(s) will be appended', {
                count: parsedModels.length,
              })
            : t('Paste model names to append them to every selected channel.')}
        </p>
      </div>
    </Dialog>
  )
}
