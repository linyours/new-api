/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export function formatQuotaRatio(ratio: number): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return '0%'
  return `${(ratio * 100).toFixed(2)}%`
}

export function formatCreatorDisplayName(
  createdBy: number,
  username: string,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (createdBy <= 0) {
    return t('Unknown creator')
  }
  if (username) {
    return username
  }
  return t('Deleted user (#{{id}})', { id: createdBy })
}
