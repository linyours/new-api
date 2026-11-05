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
import { useEffect, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Switch } from '@/components/ui/switch'

import {
  getCurrentErrorLogRetainTask,
  getCurrentErrorLogTruncateTask,
  getSystemTask,
  startErrorLogRetainTask,
  startErrorLogTruncateTask,
} from '../api'
import {
  SettingsControlGroup,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import type { LogCleanupTask } from '../types'
import type { LogSettingsFormValues } from './log-settings-section'

function isActiveLogCleanupTask(task: LogCleanupTask | null) {
  return task?.status === 'pending' || task?.status === 'running'
}

export function ErrorLogMaintenance() {
  const { t } = useTranslation()
  const form = useFormContext<LogSettingsFormValues>()
  const [showTruncateConfirm, setShowTruncateConfirm] = useState(false)
  const [showRetainConfirm, setShowRetainConfirm] = useState(false)
  const [isStartingErrorLogTask, setIsStartingErrorLogTask] = useState(false)
  const [errorLogTask, setErrorLogTask] = useState<LogCleanupTask | null>(null)
  useEffect(() => {
    let cancelled = false

    async function fetchCurrentErrorLogTask() {
      try {
        const [truncateRes, retainRes] = await Promise.all([
          getCurrentErrorLogTruncateTask(),
          getCurrentErrorLogRetainTask(),
        ])
        if (cancelled) return
        if (truncateRes.success && truncateRes.data) {
          setErrorLogTask(truncateRes.data)
          return
        }
        if (retainRes.success && retainRes.data) {
          setErrorLogTask(retainRes.data)
        }
      } catch {
        /* ignore */
      }
    }

    fetchCurrentErrorLogTask()

    return () => {
      cancelled = true
    }
  }, [])

  const errorLogTaskActive = isActiveLogCleanupTask(errorLogTask)
  const errorLogTaskState = errorLogTask?.state
  const errorLogTaskProgress = Math.min(
    100,
    Math.max(0, errorLogTaskState?.progress ?? 0)
  )
  const errorLogTaskProcessed = errorLogTaskState?.processed ?? 0
  const errorLogTaskTotal = errorLogTaskState?.total ?? 0
  const errorLogTaskId = errorLogTask?.task_id
  const retainDaysValue = form.watch('retainDays')

  useEffect(() => {
    if (!errorLogTaskId || !errorLogTaskActive) return

    let cancelled = false
    const interval = window.setInterval(async () => {
      try {
        const res = await getSystemTask(errorLogTaskId)
        if (cancelled || !res.success || !res.data) return

        setErrorLogTask(res.data)
        if (!isActiveLogCleanupTask(res.data)) {
          if (res.data.status === 'succeeded') {
            const count =
              res.data.result?.deleted_count ?? res.data.state?.processed ?? 0
            toast.success(
              count > 0
                ? t('{{count}} error log entries removed.', { count })
                : t('No error log entries needed cleanup.')
            )
          } else if (res.data.status === 'failed') {
            toast.error(res.data.error || t('Failed to clear error logs'))
          }
        }
      } catch {
        /* keep polling */
      }
    }, 1000)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [errorLogTaskActive, errorLogTaskId, t])

  const handleTruncateErrorLogs = async () => {
    setIsStartingErrorLogTask(true)
    try {
      const res = await startErrorLogTruncateTask()
      if (!res.success || res.data == null) {
        throw new Error(res.message || t('Failed to clear error logs'))
      }
      const task = res.data
      setErrorLogTask(task)
      setShowTruncateConfirm(false)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to clear error logs')
      )
    } finally {
      setIsStartingErrorLogTask(false)
    }
  }

  const handleRetainErrorLogs = async () => {
    const retainDays = form.getValues('retainDays')
    if (!Number.isInteger(retainDays) || retainDays < 1 || retainDays > 365) {
      toast.error(t('Keep error logs from the last 1 to 365 days.'))
      return
    }
    setIsStartingErrorLogTask(true)
    try {
      const res = await startErrorLogRetainTask(retainDays)
      if (!res.success || res.data == null) {
        throw new Error(res.message || t('Failed to retain error logs'))
      }
      const task = res.data
      setErrorLogTask(task)
      setShowRetainConfirm(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to retain error logs')
      )
    } finally {
      setIsStartingErrorLogTask(false)
    }
  }

  return (
    <>
      <SettingsControlGroup className='space-y-3'>
        <div>
          <h4 className='text-sm font-medium'>{t('Error log maintenance')}</h4>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Error logs are stored separately from usage logs. Clearing them reclaims table space without touching usage history.'
            )}
          </p>
        </div>
        <FormField
          control={form.control}
          name='autoCleanupEnabled'
          render={({ field }) => (
            <SettingsSwitchItem>
              <SettingsSwitchContent>
                <FormLabel>
                  {t('Automatically remove old error logs')}
                </FormLabel>
                <FormDescription>
                  {t(
                    'Delete error logs older than the retention period once a day.'
                  )}
                </FormDescription>
              </SettingsSwitchContent>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormMessage />
            </SettingsSwitchItem>
          )}
        />
        <FormField
          control={form.control}
          name='retainDays'
          render={({ field }) => (
            <FormItem className='max-w-xs'>
              <FormLabel>{t('Days to keep')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={365}
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormDescription>
                {t('Keep error logs from the last 1 to 365 days.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='flex flex-wrap gap-3'>
          <Button
            type='button'
            variant='destructive'
            onClick={() => setShowTruncateConfirm(true)}
            disabled={isStartingErrorLogTask || errorLogTaskActive}
          >
            {isStartingErrorLogTask || errorLogTaskActive
              ? t('Cleaning...')
              : t('Clear error logs')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => setShowRetainConfirm(true)}
            disabled={isStartingErrorLogTask || errorLogTaskActive}
          >
            {t('Delete older error logs')}
          </Button>
        </div>
        {errorLogTask && (
          <div className='rounded-md border p-3'>
            <div className='mb-2 flex items-center justify-between gap-3 text-sm'>
              <span className='font-medium'>
                {t('Error log cleanup progress')}
              </span>
              <span className='text-muted-foreground tabular-nums'>
                {errorLogTaskProgress}%
              </span>
            </div>
            <Progress value={errorLogTaskProgress} />
            <div className='text-muted-foreground mt-2 text-xs'>
              {t('{{processed}} of {{total}} error log entries processed.', {
                processed: errorLogTaskProcessed,
                total: errorLogTaskTotal,
              })}
            </div>
            {errorLogTask.status === 'failed' && errorLogTask.error && (
              <div className='text-destructive mt-2 text-xs'>
                {errorLogTask.error}
              </div>
            )}
          </div>
        )}
      </SettingsControlGroup>
      <ConfirmDialog
        open={showTruncateConfirm}
        onOpenChange={setShowTruncateConfirm}
        title={t('Confirm error log cleanup')}
        desc={t(
          'This will permanently delete every error log. This action cannot be undone.'
        )}
        confirmText={
          isStartingErrorLogTask ? t('Cleaning...') : t('Delete all')
        }
        destructive
        isLoading={isStartingErrorLogTask}
        handleConfirm={handleTruncateErrorLogs}
      />
      <ConfirmDialog
        open={showRetainConfirm}
        onOpenChange={setShowRetainConfirm}
        title={t('Confirm error log retention')}
        desc={t(
          'Error logs older than {{days}} days will be deleted. This action cannot be undone.',
          { days: retainDaysValue }
        )}
        confirmText={
          isStartingErrorLogTask
            ? t('Cleaning...')
            : t('Delete older error logs')
        }
        destructive
        isLoading={isStartingErrorLogTask}
        handleConfirm={handleRetainErrorLogs}
      />
    </>
  )
}
