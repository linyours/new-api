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
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export type ModelRpmLimitRow = {
  id: number
  model: string
  rpm: string
}

type ModelRpmLimitsEditorProps = {
  rows: ModelRpmLimitRow[]
  models: string[]
  disabled?: boolean
  datalistId: string
  onChange: (rows: ModelRpmLimitRow[]) => void
  onAdd: () => void
}

export function ModelRpmLimitsEditor(props: ModelRpmLimitsEditorProps) {
  const { t } = useTranslation()

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between gap-2'>
        <div>
          <Label>{t('Per-model RPM limits')}</Label>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Configured models are capped by both the model RPM and the default per-key RPM. Unconfigured models only use the default.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onAdd}
          disabled={props.disabled}
        >
          <Plus className='h-4 w-4' aria-hidden='true' />
          {t('Add model limit')}
        </Button>
      </div>
      <datalist id={props.datalistId}>
        {props.models.map((model) => (
          <option key={model} value={model} />
        ))}
      </datalist>
      {props.rows.length === 0 ? (
        <p className='text-muted-foreground rounded-md border border-dashed p-3 text-center text-xs'>
          {t('No model-specific RPM limits configured.')}
        </p>
      ) : (
        <div className='space-y-2'>
          {props.rows.map((row, index) => (
            <div
              key={row.id}
              className='grid grid-cols-[minmax(0,1fr)_7rem_auto] items-center gap-2'
            >
              <Input
                value={row.model}
                list={props.datalistId}
                placeholder={t('Model name')}
                aria-label={t('Model name')}
                disabled={props.disabled}
                onChange={(event) => {
                  const model = event.target.value
                  props.onChange(
                    props.rows.map((item, rowIndex) =>
                      rowIndex === index ? { ...item, model } : item
                    )
                  )
                }}
              />
              <Input
                type='number'
                min={0}
                step={1}
                value={row.rpm}
                placeholder='RPM'
                aria-label={t('RPM limit')}
                disabled={props.disabled}
                onChange={(event) => {
                  const rpm = event.target.value
                  props.onChange(
                    props.rows.map((item, rowIndex) =>
                      rowIndex === index ? { ...item, rpm } : item
                    )
                  )
                }}
              />
              <Button
                type='button'
                variant='ghost'
                size='icon-sm'
                className='text-destructive hover:text-destructive'
                aria-label={t('Delete model limit')}
                onClick={() =>
                  props.onChange(
                    props.rows.filter((_, rowIndex) => rowIndex !== index)
                  )
                }
                disabled={props.disabled}
              >
                <Trash2 className='h-4 w-4' aria-hidden='true' />
              </Button>
            </div>
          ))}
        </div>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Enter 0 for no extra per-model cap. The default per-key RPM still applies.'
        )}
      </p>
    </div>
  )
}
