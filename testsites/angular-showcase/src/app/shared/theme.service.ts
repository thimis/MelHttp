import { DOCUMENT, Injectable, effect, inject, signal } from '@angular/core';

const STORAGE_KEY = 'melhttp-showcase-theme';

/** Light/dark theme state. Toggles the `dark-theme` class on <body> and persists the choice. */
@Injectable({ providedIn: 'root' })
export class ThemeService {
  private readonly document = inject(DOCUMENT);
  readonly dark = signal<boolean>(this.readInitial());

  constructor() {
    effect(() => {
      const dark = this.dark();
      const body = this.document.body;
      body.classList.toggle('dark-theme', dark);
      body.classList.toggle('light-theme', !dark);
      this.persist(dark);
    });
  }

  toggle(): void {
    this.dark.update((d) => !d);
  }

  private readInitial(): boolean {
    try {
      const stored = globalThis.localStorage?.getItem(STORAGE_KEY);
      if (stored === 'dark') return true;
      if (stored === 'light') return false;
    } catch {
      // Storage may be unavailable (private mode, sandboxed iframe); fall through.
    }
    try {
      return globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
    } catch {
      return false;
    }
  }

  private persist(dark: boolean): void {
    try {
      globalThis.localStorage?.setItem(STORAGE_KEY, dark ? 'dark' : 'light');
    } catch {
      // Ignore: persistence is a convenience only.
    }
  }
}
