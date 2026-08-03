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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FormLabel } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  getChannelDisableMonitorConfig,
  updateChannelDisableMonitorConfig,
} from '../api'
import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import type { MonitorNotifyType } from '../types'

type MonitorFormState = {
  enabled: boolean
  notify_type: MonitorNotifyType
  webhook_url: string
  webhook_secret: string
}

const DEFAULT_FORM: MonitorFormState = {
  enabled: false,
  notify_type: 'root_notify',
  webhook_url: '',
  webhook_secret: '',
}

export function ChannelDisableMonitoringSection() {
  const { t } = useTranslation()
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<MonitorFormState>(DEFAULT_FORM)
  const [baseline, setBaseline] = useState<MonitorFormState>(DEFAULT_FORM)

  const configQuery = useQuery({
    queryKey: ['channel-disable-monitor-config'],
    queryFn: async () => {
      const res = await getChannelDisableMonitorConfig()
      if (!res.success) {
        throw new Error(
          res.message || t('Failed to load channel auto-disable monitoring settings')
        )
      }
      return res.data
    },
  })

  useEffect(() => {
    if (!configQuery.data) return
    const data = configQuery.data
    const next: MonitorFormState = {
      enabled: !!data.enabled,
      notify_type: data.notify_type === 'webhook' ? 'webhook' : 'root_notify',
      webhook_url: data.webhook_url || '',
      webhook_secret: '',
    }
    setForm(next)
    setBaseline(structuredClone(next))
  }, [configQuery.data])

  const onSubmit = async () => {
    if (!configQuery.isSuccess) {
      toast.warning(
        t(
          'Channel auto-disable monitoring settings have not finished loading. Please try again later.'
        )
      )
      return
    }
    if (JSON.stringify(form) === JSON.stringify(baseline)) {
      toast.info(t('No changes to save'))
      return
    }

    const payload = {
      enabled: !!form.enabled,
      notify_type: form.notify_type || 'root_notify',
      webhook_url: form.webhook_url || '',
      webhook_secret: form.webhook_secret || '',
    }

    setSubmitting(true)
    try {
      const res = await updateChannelDisableMonitorConfig(payload)
      if (!res.success) {
        throw new Error(res.message || t('Failed to update setting'))
      }
      toast.success(t('Saved successfully'))
      await configQuery.refetch()
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to update setting')
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <SettingsSection title={t('Channel auto-disable alerts')}>
      <Alert className='mb-4'>
        <AlertDescription>
          {t(
            'Alert when any channel is automatically disabled by the system. Choose root notification settings or a direct webhook.'
          )}
        </AlertDescription>
      </Alert>

      {configQuery.isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Loading...')}
        </div>
      )}
      {configQuery.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {configQuery.error instanceof Error
              ? configQuery.error.message
              : t('Failed to load channel auto-disable monitoring settings')}
          </AlertDescription>
        </Alert>
      )}
      {configQuery.isSuccess && (
        <div className='space-y-4'>
          <div className='grid gap-4 md:grid-cols-2'>
            <SettingsSwitchItem>
              <SettingsSwitchContent>
                <FormLabel>{t('Enable channel auto-disable alerts')}</FormLabel>
              </SettingsSwitchContent>
              <Switch
                checked={form.enabled}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({ ...prev, enabled: checked }))
                }
              />
            </SettingsSwitchItem>

            <div className='grid gap-1.5'>
              <FormLabel>{t('Alert method')}</FormLabel>
              <Select
                items={[
                  {
                    value: 'root_notify',
                    label: t('Use root notification settings'),
                  },
                  { value: 'webhook', label: t('Direct webhook') },
                ]}
                value={form.notify_type}
                onValueChange={(value) => {
                  if (value !== 'root_notify' && value !== 'webhook') return
                  setForm((prev) => ({ ...prev, notify_type: value }))
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='root_notify'>
                      {t('Use root notification settings')}
                    </SelectItem>
                    <SelectItem value='webhook'>{t('Direct webhook')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>

          {form.notify_type === 'webhook' ? (
            <div className='grid gap-4 md:grid-cols-2'>
              <div className='grid gap-1.5'>
                <FormLabel>{t('Webhook URL')}</FormLabel>
                <Input
                  value={form.webhook_url}
                  placeholder='https://example.com/webhook'
                  onChange={(event) =>
                    setForm((prev) => ({
                      ...prev,
                      webhook_url: event.target.value,
                    }))
                  }
                />
              </div>
              <div className='grid gap-1.5'>
                <FormLabel>{t('Webhook secret')}</FormLabel>
                <Input
                  type='password'
                  value={form.webhook_secret}
                  placeholder={t('Optional, used for signing')}
                  onChange={(event) =>
                    setForm((prev) => ({
                      ...prev,
                      webhook_secret: event.target.value,
                    }))
                  }
                />
              </div>
            </div>
          ) : null}

          <Button onClick={onSubmit} disabled={submitting}>
            {submitting ? (
              <>
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                {t('Saving...')}
              </>
            ) : (
              t('Save channel auto-disable alert settings')
            )}
          </Button>
        </div>
      )}
    </SettingsSection>
  )
}
