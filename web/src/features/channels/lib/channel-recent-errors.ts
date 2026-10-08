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
import type { TFunction } from 'i18next'

import type { StatusBadgeProps } from '@/components/status-badge'

import type { Channel } from '../types'
import { isTagAggregateRow } from './channel-utils'

export function canInspectChannelRecentErrors(channel: Channel): boolean {
  return (
    !isTagAggregateRow(channel) &&
    Number.isInteger(channel.id) &&
    channel.id > 0
  )
}

export function successRateBadgeVariant(
  successRate: number,
  alertBelow: number
): StatusBadgeProps['variant'] {
  if (successRate < alertBelow) {
    return 'danger'
  }
  if (successRate < 0.8) {
    return 'warning'
  }
  return 'success'
}

export function recentErrorStatusBadgeVariant(
  statusCode: number
): 'destructive' | 'warning' | 'outline' {
  if (statusCode >= 500) {
    return 'destructive'
  }
  if (statusCode >= 400) {
    return 'warning'
  }
  return 'outline'
}

export function successRateHeaderTint(
  variant: StatusBadgeProps['variant']
): string {
  if (variant === 'danger') {
    return 'from-destructive/12 to-destructive/4'
  }
  if (variant === 'warning') {
    return 'from-warning/12 to-warning/4'
  }
  return 'from-success/12 to-success/4'
}

export function recentErrorAccentClass(statusCode: number): string {
  if (statusCode >= 500) {
    return 'bg-destructive'
  }
  if (statusCode >= 400) {
    return 'bg-warning'
  }
  return 'bg-border'
}

export function successRateBarClass(
  variant: StatusBadgeProps['variant']
): string {
  if (variant === 'danger') {
    return 'bg-destructive'
  }
  if (variant === 'warning') {
    return 'bg-warning'
  }
  return 'bg-success'
}

export function channelHealthLayerLabel(minutes: number, t: TFunction): string {
  if (minutes === 60) {
    return t('1 hour')
  }
  if (minutes === 1) {
    return t('1 min')
  }
  return t('{{count}} min', { count: minutes })
}
