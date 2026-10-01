import { Component, inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MAT_DIALOG_DATA, MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

export interface DemoDialogData {
  title: string;
  opcodes: string[];
}

export type DemoDialogResult = 'confirmed' | undefined;

@Component({
  selector: 'app-demo-dialog',
  imports: [MatDialogModule, MatButtonModule, MatIconModule],
  template: `
    <h2 mat-dialog-title>{{ data.title }}</h2>
    <mat-dialog-content>
      <p>Malbolge has exactly eight instructions. Each one is decoded from the program cell and its address:</p>
      <ul class="ops">
        @for (op of data.opcodes; track op) {
          <li><code>{{ op }}</code></li>
        }
      </ul>
    </mat-dialog-content>
    <mat-dialog-actions align="end">
      <button matButton type="button" mat-dialog-close>Close</button>
      <button matButton="filled" type="button" (click)="confirm()">
        <mat-icon>check</mat-icon>
        Got it
      </button>
    </mat-dialog-actions>
  `,
  styles: `
    .ops {
      columns: 2;
      padding-left: 20px;
    }
  `,
})
export class DemoDialog {
  protected readonly data = inject<DemoDialogData>(MAT_DIALOG_DATA);
  private readonly ref = inject<MatDialogRef<DemoDialog, DemoDialogResult>>(MatDialogRef);

  protected confirm(): void {
    this.ref.close('confirmed');
  }
}
