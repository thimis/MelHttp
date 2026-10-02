import { Component, inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { Router, RouterLink } from '@angular/router';

@Component({
  selector: 'app-not-found',
  imports: [RouterLink, MatButtonModule, MatIconModule],
  template: `
    <section class="nf">
      <mat-icon class="big" aria-hidden="true">explore_off</mat-icon>
      <h1>Page not found</h1>
      <p class="page-intro">
        Nothing lives at <code>{{ url }}</code>. Even a Malbolge program can't print a page that does not exist.
      </p>
      <a matButton="filled" routerLink="/dashboard">Back to the dashboard</a>
    </section>
  `,
  styles: `
    .nf {
      text-align: center;
      padding: 48px 0;
    }
    .big {
      font-size: 64px;
      width: 64px;
      height: 64px;
      color: var(--mat-sys-primary);
    }
    .page-intro {
      margin-inline: auto;
    }
  `,
})
export class NotFound {
  protected readonly url = inject(Router).url;
}
