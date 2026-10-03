import { DecimalPipe } from '@angular/common';
import { AfterViewInit, Component, viewChild } from '@angular/core';
import { MatChipsModule } from '@angular/material/chips';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatPaginator, MatPaginatorModule } from '@angular/material/paginator';
import { MatSort, MatSortModule } from '@angular/material/sort';
import { MatTableDataSource, MatTableModule } from '@angular/material/table';
import { SITE_ROWS, SiteRow } from '../../shared/mock-data';
import { StatCard } from '../../shared/stat-card';

interface Stat {
  icon: string;
  label: string;
  value: string;
  trend: number;
}

@Component({
  selector: 'app-dashboard',
  imports: [
    DecimalPipe,
    StatCard,
    MatTableModule,
    MatSortModule,
    MatPaginatorModule,
    MatFormFieldModule,
    MatInputModule,
    MatIconModule,
    MatChipsModule,
  ],
  templateUrl: './dashboard.html',
  styleUrl: './dashboard.scss',
})
export class Dashboard implements AfterViewInit {
  protected readonly displayedColumns = ['id', 'name', 'framework', 'files', 'steps', 'status', 'updated'];
  protected readonly dataSource = new MatTableDataSource<SiteRow>([...SITE_ROWS]);

  private readonly sort = viewChild.required(MatSort);
  private readonly paginator = viewChild.required(MatPaginator);

  protected readonly stats: Stat[] = buildStats(SITE_ROWS);

  ngAfterViewInit(): void {
    this.dataSource.sort = this.sort();
    this.dataSource.paginator = this.paginator();
  }

  protected applyFilter(event: Event): void {
    const value = (event.target as HTMLInputElement).value;
    this.dataSource.filter = value.trim().toLowerCase();
    this.dataSource.paginator?.firstPage();
  }
}

function buildStats(rows: readonly SiteRow[]): Stat[] {
  const totalSteps = rows.reduce((sum, r) => sum + r.steps, 0);
  const files = rows.reduce((sum, r) => sum + r.files, 0);
  const healthy = rows.filter((r) => r.status === 'Healthy').length;
  return [
    { icon: 'public', label: 'Sites served', value: String(rows.length), trend: 8.3 },
    { icon: 'description', label: 'Files converted', value: files.toLocaleString('en-US'), trend: 12.1 },
    {
      icon: 'speed',
      label: 'Malbolge steps (M)',
      value: (totalSteps / 1_000_000).toFixed(1),
      trend: -2.4,
    },
    {
      icon: 'favorite',
      label: 'Healthy',
      value: `${Math.round((healthy / rows.length) * 100)}%`,
      trend: 0,
    },
  ];
}
