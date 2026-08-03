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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { TagInput } from '@/components/tag-input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import { getTTFTMonitorConfig, updateTTFTMonitorConfig } from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import type { MonitorNotifyType } from '../types'
import { safeNumberFieldProps } from '../utils/numeric-field'

const timeoutSchema = z.object({
  TTFTTimeoutSeconds: z.coerce.number().int().min(0),
})

type TimeoutFormInput = z.input<typeof timeoutSchema>
type TimeoutFormValues = z.output<typeof timeoutSchema>

type TTFTMonitoringSectionProps = {
  defaultTimeoutSeconds: number
}

type MonitorFormState = {
  enabled: boolean
  channel_ids: string[]
  threshold_seconds: number
  window_seconds: number
  count_threshold: number
  cooldown_seconds: number
  notify_type: MonitorNotifyType
  webhook_url: string
  webhook_secret: string
}

const DEFAULT_MONITOR_FORM: MonitorFormState = {
  enabled: false,
  channel_ids: [],
  threshold_seconds: 60,
  window_seconds: 60,
  count_threshold: 10,
  cooldown_seconds: 300,
  notify_type: 'root_notify',
  webhook_url: '',
  webhook_secret: '',
}

function parseChannelIDs(values: string[]): number[] {
  if (values.length === 0) return []
  const seen = new Set<number>()
  const result: number[] = []
  for (const item of values) {
    for (const part of String(item)
      .split(/[\s,，]+/)
      .map((value) => value.trim())
      .filter(Boolean)) {
      const id = Number.parseInt(part, 10)
      if (Number.isInteger(id) && id > 0 && !seen.has(id)) {
        seen.add(id)
        result.push(id)
      }
    }
  }
  return result
}

export function TTFTMonitoringSection({
  defaultTimeoutSeconds,
}: TTFTMonitoringSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<MonitorFormState>(DEFAULT_MONITOR_FORM)
  const [baseline, setBaseline] = useState<MonitorFormState>(DEFAULT_MONITOR_FORM)

  const timeoutDefaults: TimeoutFormInput = {
    TTFTTimeoutSeconds: defaultTimeoutSeconds,
  }
  const timeoutForm = useForm<TimeoutFormInput, unknown, TimeoutFormValues>({
    resolver: zodResolver(timeoutSchema),
    defaultValues: timeoutDefaults,
  })
  useResetForm(timeoutForm, timeoutDefaults)

  const configQuery = useQuery({
    queryKey: ['ttft-monitor-config'],
    queryFn: async () => {
      const res = await getTTFTMonitorConfig()
      if (!res.success) {
        throw new Error(res.message || t('Failed to load TTFT monitoring settings'))
      }
      return res.data
    },
  })

  useEffect(() => {
    if (!configQuery.data) return
    const data = configQuery.data
    const next: MonitorFormState = {
      enabled: !!data.enabled,
      channel_ids: Array.isArray(data.channel_ids)
        ? data.channel_ids.map((id) => String(id))
        : [],
      threshold_seconds: data.threshold_seconds || 60,
      window_seconds: data.window_seconds || 60,
      count_threshold: data.count_threshold || 10,
      cooldown_seconds: data.cooldown_seconds || 300,
      notify_type: data.notify_type === 'webhook' ? 'webhook' : 'root_notify',
      webhook_url: data.webhook_url || '',
      webhook_secret: '',
    }
    setForm(next)
    setBaseline(structuredClone(next))
  }, [configQuery.data])

  const onSaveTimeout = async (values: TimeoutFormValues) => {
    if (values.TTFTTimeoutSeconds === defaultTimeoutSeconds) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({
      key: 'TTFTTimeoutSeconds',
      value: values.TTFTTimeoutSeconds,
    })
  }

  const onSaveMonitor = async () => {
    if (!configQuery.isSuccess) {
      toast.warning(
        t('TTFT monitoring settings have not finished loading. Please try again later.')
      )
      return
    }
    if (JSON.stringify(form) === JSON.stringify(baseline)) {
      toast.info(t('No changes to save'))
      return
    }

    const payload = {
      enabled: !!form.enabled,
      channel_ids: parseChannelIDs(form.channel_ids),
      threshold_seconds: form.threshold_seconds || 60,
      window_seconds: form.window_seconds || 60,
      count_threshold: form.count_threshold || 10,
      cooldown_seconds: form.cooldown_seconds || 300,
      notify_type: form.notify_type || 'root_notify',
      webhook_url: form.webhook_url || '',
      webhook_secret: form.webhook_secret || '',
    }

    setSubmitting(true)
    try {
      const res = await updateTTFTMonitorConfig(payload)
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
    <SettingsSection title={t('TTFT Monitoring')}>
      <Form {...timeoutForm}>
        <SettingsForm onSubmit={timeoutForm.handleSubmit(onSaveTimeout)}>
          <SettingsPageFormActions
            onSave={timeoutForm.handleSubmit(onSaveTimeout)}
            isSaving={updateOption.isPending}
            saveLabel='Save first-token timeout'
          />
          <FormField
            control={timeoutForm.control}
            name='TTFTTimeoutSeconds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('First-token timeout (seconds)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Maximum time to wait for the first token on streaming requests. 0 disables this timeout.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      <Separator />

      <Alert>
        <AlertDescription>
          {t(
            'TTFT monitoring alerts when monitored channels exceed the slow first-token threshold within the configured window.'
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
              : t('Failed to load TTFT monitoring settings')}
          </AlertDescription>
        </Alert>
      )}
      {configQuery.isSuccess && (
        <div className='space-y-4'>
          <div className='grid gap-4 md:grid-cols-2'>
            <SettingsSwitchItem>
              <SettingsSwitchContent>
                <FormLabel>{t('Enable TTFT monitoring')}</FormLabel>
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

          <div className='grid gap-4 md:grid-cols-3'>
            <div className='grid gap-1.5'>
              <FormLabel>{t('Slow first-token threshold')}</FormLabel>
              <div className='flex items-center gap-2'>
                <Input
                  type='number'
                  min={1}
                  step={1}
                  value={form.threshold_seconds}
                  onChange={(event) => {
                    const next = event.target.valueAsNumber
                    if (Number.isFinite(next)) {
                      setForm((prev) => ({
                        ...prev,
                        threshold_seconds: next,
                      }))
                    }
                  }}
                />
                <span className='text-muted-foreground text-sm'>
                  {t('seconds')}
                </span>
              </div>
            </div>
            <div className='grid gap-1.5'>
              <FormLabel>{t('Statistics window')}</FormLabel>
              <div className='flex items-center gap-2'>
                <Input
                  type='number'
                  min={1}
                  step={1}
                  value={form.window_seconds}
                  onChange={(event) => {
                    const next = event.target.valueAsNumber
                    if (Number.isFinite(next)) {
                      setForm((prev) => ({ ...prev, window_seconds: next }))
                    }
                  }}
                />
                <span className='text-muted-foreground text-sm'>
                  {t('seconds')}
                </span>
              </div>
            </div>
            <div className='grid gap-1.5'>
              <FormLabel>{t('Trigger count')}</FormLabel>
              <Input
                type='number'
                min={1}
                step={1}
                value={form.count_threshold}
                onChange={(event) => {
                  const next = event.target.valueAsNumber
                  if (Number.isFinite(next)) {
                    setForm((prev) => ({ ...prev, count_threshold: next }))
                  }
                }}
              />
            </div>
          </div>

          <div className='grid gap-4 md:grid-cols-3'>
            <div className='grid gap-1.5'>
              <FormLabel>{t('Alert cooldown')}</FormLabel>
              <div className='flex items-center gap-2'>
                <Input
                  type='number'
                  min={1}
                  step={1}
                  value={form.cooldown_seconds}
                  onChange={(event) => {
                    const next = event.target.valueAsNumber
                    if (Number.isFinite(next)) {
                      setForm((prev) => ({ ...prev, cooldown_seconds: next }))
                    }
                  }}
                />
                <span className='text-muted-foreground text-sm'>
                  {t('seconds')}
                </span>
              </div>
            </div>
            <div className='grid gap-1.5 md:col-span-2'>
              <FormLabel>{t('Monitored channel IDs')}</FormLabel>
              <TagInput
                value={form.channel_ids}
                onChange={(channel_ids) =>
                  setForm((prev) => ({ ...prev, channel_ids }))
                }
                placeholder={t('e.g. 1, 2, 3')}
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Only channels listed here are monitored. Press Enter to add.'
                )}
              </p>
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

          <Button onClick={onSaveMonitor} disabled={submitting}>
            {submitting ? (
              <>
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                {t('Saving...')}
              </>
            ) : (
              t('Save TTFT monitoring settings')
            )}
          </Button>
        </div>
      )}
    </SettingsSection>
  )
}
