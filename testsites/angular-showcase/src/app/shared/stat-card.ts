import { Component, computed, input } from '@angular/core';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';

/** Reusable KPI tile: icon, label, value and a signed trend percentage. */
@Component({
  selector: 'app-stat-card',
  imports: [MatCardModule, MatIconModule],
  template: `
    <mat-card appearance="outlined" class="stat-card">
      <mat-card-content>
        <div class="top">
          <span class="label">{{ label() }}</span>
          <mat-icon class="icon" aria-hidden="true">{{ icon() }}</mat-icon>
        </div>
        <div class="value" data-testid="stat-value">{{ value() }}</div>
        <div class="trend" [class.up]="trend() > 0" [class.down]="trend() < 0">
          <mat-icon aria-hidden="true">{{ trendIcon() }}</mat-icon>
          <span>{{ trendText() }}</span>
          <span class="muted">vs last week</span>
        </div>
      </mat-card-content>
    </mat-card>
  `,
  styles: `
    :host {
      display: block;
    }
    .stat-card {
      height: 100%;
    }
    .top {
      display: flex;
      align-items: center;
      justify-content: space-between;
      color: var(--mat-sys-on-surface-variant);
      font: var(--mat-sys-label-large);
    }
    .icon {
      color: var(--mat-sys-primary);
    }
    .value {
      font: var(--mat-sys-headline-large);
      margin: 8px 0 4px;
    }
    .trend {
      display: flex;
      align-items: center;
      gap: 4px;
      font: var(--mat-sys-body-small);
    }
    .trend mat-icon {
      font-size: 18px;
      width: 18px;
      height: 18px;
    }
    .trend.up {
      color: light-dark(#1b6e2b, #8bd99a);
    }
    .trend.down {
      color: var(--mat-sys-error);
    }
    .muted {
      color: var(--mat-sys-on-surface-variant);
      margin-left: 4px;
    }
  `,
})
export class StatCard {
  readonly icon = input.required<string>();
  readonly label = input.required<string>();
  readonly value = input.required<string | number>();
  /** Percentage change, e.g. 4.2 or -1.5. */
  readonly trend = input<number>(0);

  protected readonly trendIcon = computed(() =>
    this.trend() > 0 ? 'trending_up' : this.trend() < 0 ? 'trending_down' : 'trending_flat',
  );
  protected readonly trendText = computed(() => {
    const t = this.trend();
    return `${t > 0 ? '+' : ''}${t.toFixed(1)}%`;
  });
}
