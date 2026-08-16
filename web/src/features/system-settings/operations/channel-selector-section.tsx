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
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

/**
 * react-hook-form treats dotted `name` strings as nested paths. Flat schema
 * keys like `'channel_selector_setting.enabled'` make validation/submit diverge
 * from form state (silent no-op on save). Use a nested object in the form and
 * flatten to option keys only when persisting.
 */
const channelSelectorSchema = z.object({
  channel_selector_setting: z.object({
    enabled: z.boolean(),
    cost_settle_enabled: z.boolean(),
    explore_rate: z.coerce.number().min(0).max(1),
    explore_rate_cold: z.coerce.number().min(0).max(1),
    min_samples: z.coerce.number().int().min(0),
  }),
})

type ChannelSelectorFormInput = z.input<typeof channelSelectorSchema>
type ChannelSelectorFormValues = z.output<typeof channelSelectorSchema>

type FlatChannelSelectorDefaults = {
  'channel_selector_setting.enabled': boolean
  'channel_selector_setting.cost_settle_enabled': boolean
  'channel_selector_setting.explore_rate': number
  'channel_selector_setting.explore_rate_cold': number
  'channel_selector_setting.min_samples': number
}

type ChannelSelectorSectionProps = {
  defaultValues: FlatChannelSelectorDefaults
}

const buildFormDefaults = (
  defaults: FlatChannelSelectorDefaults
): ChannelSelectorFormInput => ({
  channel_selector_setting: {
    enabled: defaults['channel_selector_setting.enabled'],
    cost_settle_enabled: defaults['channel_selector_setting.cost_settle_enabled'],
    explore_rate: defaults['channel_selector_setting.explore_rate'],
    explore_rate_cold: defaults['channel_selector_setting.explore_rate_cold'],
    min_samples: defaults['channel_selector_setting.min_samples'],
  },
})

const flattenFormValues = (
  values: ChannelSelectorFormValues
): FlatChannelSelectorDefaults => ({
  'channel_selector_setting.enabled': values.channel_selector_setting.enabled,
  'channel_selector_setting.cost_settle_enabled':
    values.channel_selector_setting.cost_settle_enabled,
  'channel_selector_setting.explore_rate':
    values.channel_selector_setting.explore_rate,
  'channel_selector_setting.explore_rate_cold':
    values.channel_selector_setting.explore_rate_cold,
  'channel_selector_setting.min_samples':
    values.channel_selector_setting.min_samples,
})

export function ChannelSelectorSection({
  defaultValues,
}: ChannelSelectorSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<
    ChannelSelectorFormInput,
    unknown,
    ChannelSelectorFormValues
  >({
    resolver: zodResolver(channelSelectorSchema),
    defaultValues: formDefaults,
  })

  const baselineRef = useRef<FlatChannelSelectorDefaults>(defaultValues)
  const baselineSerializedRef = useRef<string>(JSON.stringify(defaultValues))

  useEffect(() => {
    const serialized = JSON.stringify(defaultValues)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = defaultValues
    baselineSerializedRef.current = serialized
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (data: ChannelSelectorFormValues) => {
    const flat = flattenFormValues(data)
    const updates = (
      Object.keys(flat) as Array<keyof FlatChannelSelectorDefaults>
    ).filter((key) => flat[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({
        key,
        value: String(flat[key]),
      })
    }

    baselineRef.current = flat
  }

  return (
    <SettingsSection title={t('Multi-factor Channel Selector')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='channel_selector_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Enable multi-factor channel selection')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, channels are chosen by cost price, success rate, and latency instead of priority/weight. Channel affinity still takes precedence.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={form.control}
            name='channel_selector_setting.cost_settle_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Enable channel cost-price settle')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, final settle quota is multiplied by channel cost_price. Pre-consume still uses the global model price.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={form.control}
            name='channel_selector_setting.explore_rate'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Explore rate')}</FormLabel>
                <FormDescription>
                  {t(
                    'Probability of random exploration instead of score-based pick (0–1). Higher values send more traffic to cold channels.'
                  )}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={1}
                    step='0.01'
                    className='max-w-xs'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='channel_selector_setting.explore_rate_cold'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Cold-start explore floor')}</FormLabel>
                <FormDescription>
                  {t(
                    'Minimum explore rate while any candidate has fewer than Min samples. Must be between 0 and 1.'
                  )}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    max={1}
                    step='0.01'
                    className='max-w-xs'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='channel_selector_setting.min_samples'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Min samples')}</FormLabel>
                <FormDescription>
                  {t(
                    'Channel×model attempts below this count are treated as cold-start.'
                  )}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    className='max-w-xs'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
