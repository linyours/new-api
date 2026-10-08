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

export function parsePastedModels(raw: string): string[] {
  const seen = new Set<string>()
  const models: string[] = []
  for (const part of raw.split(/[\n\r\t,，、]+/)) {
    const model = part.trim()
    if (!model || seen.has(model)) {
      continue
    }
    seen.add(model)
    models.push(model)
  }
  return models
}

export function parsePastedModelRpmLimits(
  raw: string
): Record<string, number> | null {
  const lines = raw
    .split(/\n/)
    .map((line) => line.trim())
    .filter(Boolean)
  if (lines.length === 0) {
    return null
  }

  const limits: Record<string, number> = {}
  for (const line of lines) {
    const parsed = parseModelRpmLine(line)
    if (parsed === null || Object.hasOwn(limits, parsed.model)) {
      return null
    }
    limits[parsed.model] = parsed.rpm
  }
  return limits
}

function parseModelRpmLine(
  line: string
): { model: string; rpm: number } | null {
  const colon = line.lastIndexOf(':')
  const comma = Math.max(line.lastIndexOf(','), line.lastIndexOf('，'))
  let model = ''
  let rpmRaw = ''

  if (colon > 0) {
    model = line.slice(0, colon).trim()
    rpmRaw = line.slice(colon + 1).trim()
  } else if (comma > 0) {
    model = line.slice(0, comma).trim()
    rpmRaw = line.slice(comma + 1).trim()
  } else {
    const match = line.match(/^(.*\S)\s+(\S+)$/)
    if (!match) {
      return null
    }
    model = match[1].trim()
    rpmRaw = match[2]
  }

  if (!model || rpmRaw === '') {
    return null
  }

  const rpm = Number(rpmRaw)
  if (!Number.isSafeInteger(rpm) || rpm < 0) {
    return null
  }

  return { model, rpm }
}
