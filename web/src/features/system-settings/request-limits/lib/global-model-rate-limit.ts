/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import * as z from 'zod'

export function parseGlobalModelRPMLimits(
  value: string
): Record<string, number> | null {
  try {
    const parsed: unknown = JSON.parse(value.trim() || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    const entries = Object.entries(parsed)
    if (entries.length > 1000) return null
    const names = new Set<string>()
    const limits: [string, number][] = []
    for (const [key, rpm] of entries) {
      const name = key.trim()
      if (
        !name ||
        new TextEncoder().encode(name).length > 255 ||
        name.includes('\r') ||
        name.includes('\n') ||
        name.includes('\0') ||
        names.has(name) ||
        typeof rpm !== 'number' ||
        !Number.isInteger(rpm) ||
        rpm < 0 ||
        rpm > 100000
      ) {
        return null
      }
      names.add(name)
      limits.push([name, rpm])
    }
    return Object.fromEntries(limits)
  } catch {
    return null
  }
}

export function createGlobalModelRateLimitSchema(t: (key: string) => string) {
  return z.object({
    enabled: z.boolean(),
    limits: z
      .string()
      .refine((value) => parseGlobalModelRPMLimits(value) !== null, {
        message: t(
          'Use unique model names and integer RPM values from 0 to 100000, with at most 1000 models.'
        ),
      }),
  })
}

export type GlobalModelRateLimitFormValues = z.infer<
  ReturnType<typeof createGlobalModelRateLimitSchema>
>
