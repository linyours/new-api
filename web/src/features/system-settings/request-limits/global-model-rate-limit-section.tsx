/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useEffectEvent } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { JsonEditor } from '@/components/json-editor'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  createGlobalModelRateLimitSchema,
  parseGlobalModelRPMLimits,
  type GlobalModelRateLimitFormValues,
} from './lib/global-model-rate-limit'

export function GlobalModelRateLimitSection(props: { value: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<GlobalModelRateLimitFormValues>({
    resolver: zodResolver(createGlobalModelRateLimitSchema(t)),
    defaultValues: { enabled: false, limits: '{}' },
    mode: 'onChange',
  })

  const loadSavedConfig = useEffectEvent(() => {
    try {
      const parsed: unknown = JSON.parse(props.value)
      if (!parsed || typeof parsed !== 'object') return
      if (
        'enabled' in parsed &&
        'limits' in parsed &&
        typeof parsed.enabled === 'boolean'
      ) {
        form.reset({
          enabled: parsed.enabled,
          limits: JSON.stringify(parsed.limits),
        })
      }
    } catch {
      form.setError('limits', {
        message: t('Invalid global model RPM configuration'),
      })
    }
  })
  useEffect(() => {
    loadSavedConfig()
  }, [props.value])

  const onSubmit = async (values: GlobalModelRateLimitFormValues) => {
    const limits = parseGlobalModelRPMLimits(values.limits)
    if (limits === null) return
    try {
      await updateOption.mutateAsync({
        key: 'GlobalModelRateLimit',
        value: JSON.stringify({ enabled: values.enabled, limits }),
      })
    } catch {
      // useUpdateOption reports the failure. Keep the editable draft.
    }
  }

  return (
    <SettingsSection title={t('Site-wide model RPM limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            isSaveDisabled={!form.formState.isValid}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Enable site-wide model RPM limits')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Each model shares one rolling 60-second limit across all users, tokens, groups, and channels.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <FormField
            control={form.control}
            name='limits'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Model RPM limits')}</FormLabel>
                <FormControl>
                  <div role='group' aria-label={t('Model RPM limits')}>
                    <JsonEditor
                      key={props.value}
                      value={field.value}
                      onChange={field.onChange}
                      valueType='number'
                      strict
                      allowJsonMode={false}
                      disabled={updateOption.isPending}
                      keyLabel={t('Model name')}
                      keyPlaceholder={t('Model name')}
                      valueLabel={t('RPM limit')}
                      valuePlaceholder='RPM'
                    />
                  </div>
                </FormControl>
                <FormDescription>
                  <span className='block'>
                    {t(
                      'Match the requested model name exactly, before channel mapping. Missing models and 0 are unlimited. Failed upstream requests count; internal retries count once.'
                    )}
                  </span>
                  <span className='mt-1 block'>
                    {t(
                      'Multiple gateway instances must share Redis. Without Redis, limits apply to this instance only.'
                    )}
                  </span>
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
