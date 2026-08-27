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
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

export const MAX_CHANNEL_KEY_QUOTA_UNITS = 2147483647

function getQuotaUnitsPerUSD(): number {
  const configured =
    useSystemConfigStore.getState().config.currency.quotaPerUnit
  return Number.isFinite(configured) && configured > 0
    ? configured
    : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
}

export function channelKeyQuotaUnitsToUSD(units: number): number {
  return units / getQuotaUnitsPerUSD()
}

export function channelKeyUSDToQuotaUnits(amount: number): number {
  return Math.round(amount * getQuotaUnitsPerUSD())
}

export function isValidChannelKeyUSDLimit(amount: number): boolean {
  if (!Number.isFinite(amount) || amount < 0) return false
  const units = channelKeyUSDToQuotaUnits(amount)
  return units >= 0 && units <= MAX_CHANNEL_KEY_QUOTA_UNITS
}

export function formatChannelKeyQuotaUSD(units: number): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(channelKeyQuotaUnitsToUSD(units))
}
