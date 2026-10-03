import { DecimalPipe } from '@angular/common';
import { Component, DOCUMENT, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSelectModule } from '@angular/material/select';

interface HeaderRow {
  name: string;
  value: string | null;
}

type HeaderState =
  | { kind: 'loading' }
  | { kind: 'ok'; rows: HeaderRow[] }
  | { kind: 'error'; message: string };

type SourceState =
  | { kind: 'idle' }
  | { kind: 'loading'; path: string }
  | { kind: 'ok'; path: string; preview: string; length: number; truncated: boolean }
  | { kind: 'unavailable'; path: string; detail: string };

interface SourceOption {
  path: string;
  label: string;
}

const PREVIEW_CHARS = 4000;
const INTERESTING_HEADERS = ['X-Malbolge-Steps', 'X-Powered-By', 'Content-Type', 'Content-Length', 'Server'];

@Component({
  selector: 'app-malbolge',
  imports: [
    DecimalPipe,
    FormsModule,
    MatButtonModule,
    MatCardModule,
    MatFormFieldModule,
    MatIconModule,
    MatProgressBarModule,
    MatSelectModule,
  ],
  templateUrl: './malbolge.html',
  styleUrl: './malbolge.scss',
})
export class Malbolge implements OnInit {
  private readonly document = inject(DOCUMENT);

  protected readonly headers = signal<HeaderState>({ kind: 'loading' });
  protected readonly source = signal<SourceState>({ kind: 'idle' });
  protected readonly options = signal<SourceOption[]>([]);
  protected readonly selected = signal<string>('index.html');

  ngOnInit(): void {
    this.options.set(this.discoverSources());
    void this.loadHeaders();
  }

  protected onSelect(path: string): void {
    this.selected.set(path);
    void this.loadSource(path);
  }

  protected reload(): void {
    void this.loadSource(this.selected());
  }

  /** HEAD the site root and surface the MelHttp response headers. Never throws. */
  private async loadHeaders(): Promise<void> {
    try {
      const origin = this.document.location?.origin ?? '';
      const res = await fetch(origin + '/', { method: 'HEAD', cache: 'no-store' });
      const rows = INTERESTING_HEADERS.map((name) => ({ name, value: res.headers.get(name) }));
      this.headers.set({ kind: 'ok', rows });
    } catch (err) {
      this.headers.set({ kind: 'error', message: describe(err) });
    }
  }

  /** Fetch /_source/<path>; any failure becomes a friendly "unavailable" state. Never throws. */
  private async loadSource(path: string): Promise<void> {
    this.source.set({ kind: 'loading', path });
    try {
      const res = await fetch('/_source/' + path, { cache: 'no-store' });
      if (!res.ok) {
        this.source.set({ kind: 'unavailable', path, detail: `HTTP ${res.status}` });
        return;
      }
      const type = res.headers.get('Content-Type') ?? '';
      // A static server with SPA fallback answers unknown paths with index.html; detect that.
      if (!path.endsWith('.html') && type.includes('text/html')) {
        this.source.set({ kind: 'unavailable', path, detail: 'server returned the SPA fallback page' });
        return;
      }
      const text = await res.text();
      if (this.selected() !== path) return; // a newer request superseded this one
      this.source.set({
        kind: 'ok',
        path,
        preview: text.slice(0, PREVIEW_CHARS),
        length: text.length,
        truncated: text.length > PREVIEW_CHARS,
      });
    } catch (err) {
      this.source.set({ kind: 'unavailable', path, detail: describe(err) });
    }
  }

  /** Builds the list of viewable files from what this very page loaded. */
  private discoverSources(): SourceOption[] {
    const out: SourceOption[] = [{ path: 'index.html', label: 'index.html (document)' }];
    const seen = new Set<string>(['index.html']);
    const add = (href: string | null | undefined, kind: string) => {
      const path = sameOriginPath(href, this.document);
      if (path && !seen.has(path)) {
        seen.add(path);
        out.push({ path, label: `${path} (${kind})` });
      }
    };
    try {
      for (const s of Array.from(this.document.scripts)) {
        add(s.src, /(^|\/)main[-.]/.test(s.src) ? 'main bundle' : 'script');
      }
      for (const sheet of Array.from(this.document.styleSheets)) {
        add(sheet.href, 'styles');
      }
      for (const link of Array.from(this.document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'))) {
        add(link.href, 'styles');
      }
    } catch {
      // Discovery is best-effort.
    }
    return out;
  }
}

function sameOriginPath(href: string | null | undefined, doc: Document): string | null {
  if (!href) return null;
  try {
    const url = new URL(href, doc.baseURI);
    if (url.origin !== doc.location.origin) return null;
    const path = url.pathname.replace(/^\/+/, '');
    return path || null;
  } catch {
    return null;
  }
}

function describe(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
