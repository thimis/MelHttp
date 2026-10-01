import { Component, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatChipsModule } from '@angular/material/chips';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSnackBar } from '@angular/material/snack-bar';
import { DemoDialog, DemoDialogData, DemoDialogResult } from './demo-dialog';

interface Tile {
  title: string;
  icon: string;
  body: string;
}

@Component({
  selector: 'app-theming',
  imports: [MatButtonModule, MatCardModule, MatChipsModule, MatIconModule, MatProgressBarModule],
  templateUrl: './theming.html',
  styleUrl: './theming.scss',
})
export class Theming {
  private readonly dialog = inject(MatDialog);
  private readonly snackBar = inject(MatSnackBar);

  protected readonly lastResult = signal<string>('none yet');
  protected readonly schemes = ['light', 'dark'] as const;

  protected readonly tiles: Tile[] = [
    { title: 'Crazy operation', icon: 'shuffle', body: 'Tritwise op on two values using a fixed 3×3 table.' },
    { title: 'Rotate', icon: 'rotate_right', body: 'Rotates the 10-trit word at [d] one trit right.' },
    { title: 'Jump', icon: 'call_split', body: 'Sets the code pointer to the value at [d].' },
    { title: 'Encryption', icon: 'lock', body: 'Every executed instruction is re-encrypted in place.' },
    { title: 'Memory', icon: 'memory', body: '59,049 cells of 10 trits each — 3^10 words.' },
    { title: 'Output', icon: 'print', body: 'Opcode 5 prints A mod 256: the only way bytes leave.' },
  ];

  protected openDialog(): void {
    const ref = this.dialog.open<DemoDialog, DemoDialogData, DemoDialogResult>(DemoDialog, {
      data: {
        title: 'The eight Malbolge instructions',
        opcodes: ['j  jump d', 'i  jump c', '*  rotate', 'p  crazy op', '<  output', '/  input', 'v  halt', 'o  nop'],
      },
      width: 'min(480px, calc(100vw - 32px))',
      autoFocus: 'first-tabbable',
    });
    ref.afterClosed().subscribe((result) => this.lastResult.set(result || 'dismissed'));
  }

  protected showSnack(kind: 'info' | 'action'): void {
    if (kind === 'info') {
      this.snackBar.open('This message was printed one Malbolge step at a time.', undefined, { duration: 3000 });
    } else {
      const ref = this.snackBar.open('Toggled a trit.', 'Undo', { duration: 5000 });
      ref.onAction().subscribe(() => this.snackBar.open('Trit restored.', undefined, { duration: 2000 }));
    }
  }
}
