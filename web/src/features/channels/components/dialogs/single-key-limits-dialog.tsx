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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Skeleton } from '@/components/ui/skeleton'

import { getMultiKeyStatus, updateChannelKeyLimits } from '../../api'
import { parseModelsString } from '../../lib'
import type { KeyStatus } from '../../types'
import { useChannels } from '../channels-provider'
import { ChannelKeyLimitsDialog } from './channel-key-limits-dialog'

type SingleKeyLimitsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SingleKeyLimitsDialog(props: SingleKeyLimitsDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const [keyStatus, setKeyStatus] = useState<KeyStatus | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const models = useMemo(
    () => parseModelsString(currentRow?.models || ''),
    [currentRow?.models]
  )
  const channelId = currentRow?.id
  const dialogOpen = props.open
  const onOpenChangeRef = useRef(props.onOpenChange)
  onOpenChangeRef.current = props.onOpenChange

  useEffect(() => {
    if (!dialogOpen || channelId == null) {
      setKeyStatus(null)
      return
    }

    let cancelled = false

    const loadKeyStatus = async () => {
      setIsLoading(true)
      try {
        const response = await getMultiKeyStatus(channelId, 1, 1)
        if (cancelled) return
        if (!response.success || !response.data?.keys?.[0]) {
          toast.error(response.message || t('Failed to load key status'))
          onOpenChangeRef.current(false)
          return
        }
        setKeyStatus(response.data.keys[0])
      } catch (error: unknown) {
        if (cancelled) return
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to load key status')
        )
        onOpenChangeRef.current(false)
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }

    void loadKeyStatus()
    return () => {
      cancelled = true
    }
  }, [channelId, dialogOpen, t])

  const saveKeyLimits = async (
    rpmLimit: number | null,
    quotaLimit: number | null,
    modelRpmLimits: Record<string, number>
  ) => {
    if (!currentRow || !keyStatus) return
    setIsSaving(true)
    try {
      const response = await updateChannelKeyLimits(
        currentRow.id,
        keyStatus.id,
        rpmLimit,
        quotaLimit,
        modelRpmLimits
      )
      if (!response.success) {
        toast.error(response.message || t('Operation failed'))
        return
      }
      toast.success(response.message || t('Operation successful'))
      props.onOpenChange(false)
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setIsSaving(false)
    }
  }

  if (isLoading && !keyStatus) {
    return (
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Configure key limits')}
        description={t(
          'Leave a field empty to inherit the channel default. Enter 0 for unlimited.'
        )}
      >
        <div className='space-y-3'>
          <Skeleton className='h-10 w-full' />
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-10 w-full' />
        </div>
      </Dialog>
    )
  }

  return (
    <ChannelKeyLimitsDialog
      open={props.open && keyStatus !== null}
      keyStatus={keyStatus}
      saving={isSaving}
      models={models}
      onOpenChange={props.onOpenChange}
      onSave={saveKeyLimits}
    />
  )
}
