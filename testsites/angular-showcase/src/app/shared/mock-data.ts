export type DeployStatus = 'Healthy' | 'Degraded' | 'Building' | 'Offline';

export interface SiteRow {
  id: number;
  name: string;
  framework: string;
  files: number;
  steps: number;
  status: DeployStatus;
  updated: string;
}

const PREFIXES = ['crazy', 'trit', 'rot', 'jump', 'opr', 'load', 'nop', 'halt', 'cell', 'ternary'];
const SUFFIXES = ['blog', 'shop', 'docs', 'wiki', 'portal', 'lab', 'store', 'notes', 'zone', 'hub'];
const FRAMEWORKS = ['Angular', 'React', 'Vue', 'Svelte', 'Static HTML'];
const STATUSES: DeployStatus[] = ['Healthy', 'Healthy', 'Healthy', 'Degraded', 'Building', 'Offline'];

/** Small deterministic PRNG (mulberry32) so mock data is stable across reloads. */
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function buildRows(count: number): SiteRow[] {
  const rand = mulberry32(3 ** 10);
  const base = Date.UTC(2026, 8, 30);
  const rows: SiteRow[] = [];
  for (let i = 0; i < count; i++) {
    const pre = PREFIXES[i % PREFIXES.length];
    const suf = SUFFIXES[Math.floor(rand() * SUFFIXES.length)];
    const day = new Date(base - Math.floor(rand() * 90) * 86_400_000);
    rows.push({
      id: i + 1,
      name: `${pre}-${suf}-${String(i + 1).padStart(2, '0')}`,
      framework: FRAMEWORKS[Math.floor(rand() * FRAMEWORKS.length)],
      files: 3 + Math.floor(rand() * 120),
      steps: Math.floor(rand() * 9_000_000) + 50_000,
      status: STATUSES[Math.floor(rand() * STATUSES.length)],
      updated: day.toISOString().slice(0, 10),
    });
  }
  return rows;
}

export const SITE_ROWS: readonly SiteRow[] = buildRows(50);
