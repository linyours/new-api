/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
export type ModelRpmLimitInput = {
  model: string
  rpm: string
}

export function unusedModelName(
  models: string[],
  rows: { model: string }[]
): string {
  const used = new Set(rows.map((row) => row.model.trim()))
  return models.find((model) => !used.has(model)) || ''
}

export function parseModelRpmLimits(
  rows: ModelRpmLimitInput[]
): Record<string, number> | null {
  const limits: Record<string, number> = {}
  for (const row of rows) {
    const model = row.model.trim()
    const rpm = Number(row.rpm)
    if (
      model === '' ||
      row.rpm.trim() === '' ||
      !Number.isSafeInteger(rpm) ||
      rpm < 0 ||
      Object.hasOwn(limits, model)
    ) {
      return null
    }
    limits[model] = rpm
  }
  return limits
}
