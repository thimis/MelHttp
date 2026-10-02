# MelHttp test sites

Frontend sites used to exercise MelHttp end to end. Each one is a normal single-page app whose
production build gets converted into Malbolge programs and served by `melhttpd`. All three use
HTML5 history routing, so the server has to fall back to `index.html` for unknown paths.

| Site | Stack | Build output | Default test URL |
| --- | --- | --- | --- |
| `angular-showcase/` | Angular 22, Angular Material (M3), standalone components, zoneless | `dist/angular-showcase/browser/` | `http://localhost:4200` |
| `react-vite/` | React 19, Vite, react-router-dom (`BrowserRouter`) | `dist/` | `http://localhost:4173` |
| `vue-vite/` | Vue 3, Vite, vue-router (`createWebHistory`) | `dist/` | `http://localhost:4173` |

Nothing makes network requests to other hosts at runtime. The Angular site self-hosts its fonts
(`@fontsource/roboto` latin 400/500 and the `material-icons` icon font), so the woff/woff2 files
are in the build under `media/`.

## angular-showcase

Routes: `/` redirects to `/dashboard`, then `/dashboard`, `/forms`, `/theming`, `/malbolge`, and
`**` for not found. Pages are lazy-loaded, so the build has several JS chunks.

- **Shell**: responsive `mat-sidenav` with a toolbar. The theme toggle (`aria-label="Toggle theme"`)
  switches `dark-theme`/`light-theme` on `<body>` and saves the choice in `localStorage`.
- **Dashboard**: four `app-stat-card` components and a `mat-table` with 50 mock rows. The table
  has `MatSort`, `MatPaginator` and a "Filter" input.
- **Forms**: a linear `mat-stepper` with three steps:
  1. Personal info: required and email validators, plus `app-trit-input`, a custom
     `MatFormFieldControl` that only accepts the digits 0, 1 and 2.
  2. Preferences: datepicker, chips input, select and slide toggle.
  3. Review: a summary of the answers. Submitting shows a snackbar.
- **Theming**: a `MatDialog` with a custom dialog component, snackbars, and buttons, chips and
  cards shown in the light and dark color schemes side by side. Also a responsive grid.
- **Malbolge corner**: sends `HEAD /` and shows the `X-Malbolge-Steps` and `X-Powered-By` headers.
  A source viewer fetches `/_source/<path>` for `index.html`, the main bundle and the styles CSS.
  It shows the first 4000 characters and the total length. If the endpoint is missing, the page
  says "Source endpoint not available (start melhttpd with -expose-source)" instead.

```sh
cd testsites/angular-showcase
npm ci
npm run build                      # -> dist/angular-showcase/browser/
npx -y serve -s dist/angular-showcase/browser -l 4200   # any static server with SPA fallback
BASE_URL=http://localhost:4200 npm run e2e
```

## react-vite / vue-vite

Both sites have the same routes: `/` (a counter and a list), `/about`, and a catch-all not-found
page. Each has a nav bar and imports `src/assets/hero.png`, which ends up in the build as a hashed
binary asset. The React site also bundles `react.svg` as a separate file. The Vue site lazy-loads
its `/about` and not-found views as separate chunks.

```sh
cd testsites/react-vite            # or testsites/vue-vite
npm ci
npm run build                      # -> dist/
npx vite preview --port 4173       # has history fallback built in
BASE_URL=http://localhost:4173 npm run e2e
```

## Playwright smoke tests

- Each project has `@playwright/test` as a devDependency, a `playwright.config.ts` and tests in `e2e/`.
- The config runs chromium only, with `retries: 0` and the `line` reporter.
- The config does not start a server. Start one yourself (or from the harness) and set `BASE_URL`.
  Without `BASE_URL`, tests use the default URL from the table above.
- A shared fixture (`e2e/fixtures.ts`) fails any test that logs a `console` error or raises a
  `pageerror`.
- Install the browser once with `npx playwright install chromium`.
- The `e2e/` folders are not part of the app builds. Angular compiles only `src/**`, and Vite
  bundles only what `index.html` imports.

Angular tests cover:

- Deep links to `/dashboard` and `/forms`, including a reload.
- Table sorting, filtering and paging.
- The theme toggle, and that the choice survives a reload.
- Stepper validation, Next, and the snackbar on submit.
- Opening and closing the dialog.
- The Malbolge corner page renders, whether or not the headers and `/_source` are available.
- Nav links route without a full page reload, and unknown paths show the not-found page.

React and Vue tests cover:

- `/` renders.
- The counter increments.
- Clicking the nav link goes to `/about` without a full page reload.
- Loading `/about` directly works.
- Unknown paths show the not-found page.
- Images load (`naturalWidth > 0`).
