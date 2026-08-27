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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { MultiKeyConfirmAction } from '../../types'

type MultiKeyTableRowActionsProps = {
  keyId: number
  keyIndex: number
  status: number
  canConfigure: boolean
  canDelete: boolean
  onAction: (action: MultiKeyConfirmAction) => void
  onConfigure: (keyId: number) => void
  onResetQuota: (keyId: number) => void
}

export function MultiKeyTableRowActions(props: MultiKeyTableRowActionsProps) {
  const { t } = useTranslation()
  const isEnabled = props.status === 1
  const isQuotaExhausted = props.status === 4
  let statusAction
  if (isEnabled) {
    statusAction = (
      <Button
        variant='outline'
        size='sm'
        onClick={() =>
          props.onAction({ type: 'disable', keyIndex: props.keyIndex })
        }
      >
        {t('Disable')}
      </Button>
    )
  } else if (isQuotaExhausted) {
    statusAction = (
      <Button
        variant='outline'
        size='sm'
        onClick={() => props.onResetQuota(props.keyId)}
        disabled={!props.canConfigure}
        title={
          props.canConfigure
            ? undefined
            : t('No permission to perform this action')
        }
      >
        {t('Reset quota')}
      </Button>
    )
  } else {
    statusAction = (
      <Button
        variant='outline'
        size='sm'
        onClick={() =>
          props.onAction({ type: 'enable', keyIndex: props.keyIndex })
        }
      >
        {t('Enable')}
      </Button>
    )
  }

  return (
    <div className='flex justify-end gap-2'>
      <Button
        variant='outline'
        size='sm'
        onClick={() => props.onConfigure(props.keyId)}
        disabled={!props.canConfigure}
        title={
          props.canConfigure
            ? undefined
            : t('No permission to perform this action')
        }
      >
        {t('Limits')}
      </Button>
      {statusAction}
      <Button
        variant='destructive'
        size='sm'
        onClick={() => {
          if (!props.canDelete) return
          props.onAction({ type: 'delete', keyIndex: props.keyIndex })
        }}
        disabled={!props.canDelete}
        title={
          props.canDelete
            ? undefined
            : t('No permission to perform this action')
        }
      >
        {t('Delete')}
      </Button>
    </div>
  )
}
