import type { CollectorVersionInfo } from './types'

export type { CollectorVersionInfo }

const STORAGE_KEY = 'o11y.collector_version'

let current = ''
let defaultVersion = ''
let versions: CollectorVersionInfo[] = []

export function getCollectorVersion(): string {
  return current || defaultVersion
}

export function getCollectorVersions(): CollectorVersionInfo[] {
  return versions
}

export function getDefaultCollectorVersion(): string {
  return defaultVersion
}

export function setCollectorVersion(v: string): void {
  current = v
  try {
    localStorage.setItem(STORAGE_KEY, v)
  } catch {
    /* ignore */
  }
}

export async function loadCollectorVersions(): Promise<void> {
  const res = await fetch('/api/collector-versions')
  if (!res.ok) throw new Error('failed to load collector versions')
  const data = (await res.json()) as { default: string; versions: CollectorVersionInfo[] }
  defaultVersion = data.default
  versions = data.versions || []
  let stored = ''
  try {
    stored = localStorage.getItem(STORAGE_KEY) || ''
  } catch {
    /* ignore */
  }
  if (stored && versions.some((v) => v.contrib === stored)) current = stored
  else current = defaultVersion
}

export function withCollectorVersion(path: string, version = getCollectorVersion()): string {
  const u = new URL(path, 'http://localhost')
  if (version) u.searchParams.set('collector_version', version)
  return u.pathname + u.search
}
