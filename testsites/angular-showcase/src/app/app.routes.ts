import { Routes } from '@angular/router';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'dashboard' },
  {
    path: 'dashboard',
    title: 'Dashboard · MelHttp Showcase',
    loadComponent: () => import('./pages/dashboard/dashboard').then((m) => m.Dashboard),
  },
  {
    path: 'forms',
    title: 'Forms · MelHttp Showcase',
    loadComponent: () => import('./pages/forms/forms-page').then((m) => m.FormsPage),
  },
  {
    path: 'theming',
    title: 'Theming · MelHttp Showcase',
    loadComponent: () => import('./pages/theming/theming').then((m) => m.Theming),
  },
  {
    path: 'malbolge',
    title: 'Malbolge corner · MelHttp Showcase',
    loadComponent: () => import('./pages/malbolge/malbolge').then((m) => m.Malbolge),
  },
  {
    path: '**',
    title: 'Not found · MelHttp Showcase',
    loadComponent: () => import('./pages/not-found/not-found').then((m) => m.NotFound),
  },
];
