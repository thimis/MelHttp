import { COMMA, ENTER } from '@angular/cdk/keycodes';
import { DatePipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, inject, signal, viewChild } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatChipInputEvent, MatChipsModule } from '@angular/material/chips';
import { provideNativeDateAdapter } from '@angular/material/core';
import { MatDatepickerModule } from '@angular/material/datepicker';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';
import { MatSnackBar } from '@angular/material/snack-bar';
import { MatStepper, MatStepperModule } from '@angular/material/stepper';
import { TritInput } from './trit-input';

@Component({
  selector: 'app-forms-page',
  // Eager change detection keeps the review summary in sync with plain reactive-form values.
  changeDetection: ChangeDetectionStrategy.Eager,
  imports: [
    DatePipe,
    ReactiveFormsModule,
    MatStepperModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatIconModule,
    MatDatepickerModule,
    MatChipsModule,
    MatSelectModule,
    MatSlideToggleModule,
    TritInput,
  ],
  providers: [provideNativeDateAdapter()],
  templateUrl: './forms-page.html',
  styleUrl: './forms-page.scss',
})
export class FormsPage {
  private readonly fb = inject(FormBuilder);
  private readonly snackBar = inject(MatSnackBar);
  private readonly stepper = viewChild.required(MatStepper);

  protected readonly separatorKeys = [ENTER, COMMA] as const;
  protected readonly submitted = signal(false);
  protected readonly minDate = new Date(2026, 0, 1);

  protected readonly personal = this.fb.nonNullable.group({
    name: ['', [Validators.required, Validators.minLength(2)]],
    email: ['', [Validators.required, Validators.email]],
    trits: ['', [Validators.required, Validators.minLength(3)]],
  });

  protected readonly preferences = this.fb.group({
    launchDate: this.fb.control<Date | null>(null),
    tags: this.fb.nonNullable.control<string[]>(['ternary', 'crazy-op']),
    framework: this.fb.nonNullable.control<string>('angular'),
    newsletter: this.fb.nonNullable.control<boolean>(true),
  });

  protected readonly frameworks = [
    { value: 'angular', label: 'Angular' },
    { value: 'react', label: 'React' },
    { value: 'vue', label: 'Vue' },
    { value: 'static', label: 'Plain HTML' },
  ];

  protected frameworkLabel(value: string | null | undefined): string {
    return this.frameworks.find((f) => f.value === value)?.label ?? '—';
  }

  protected addTag(event: MatChipInputEvent): void {
    const value = (event.value || '').trim();
    if (value) {
      const tags = this.preferences.controls.tags.value;
      if (!tags.includes(value)) {
        this.preferences.controls.tags.setValue([...tags, value]);
      }
    }
    event.chipInput.clear();
  }

  protected removeTag(tag: string): void {
    const tags = this.preferences.controls.tags.value.filter((t) => t !== tag);
    this.preferences.controls.tags.setValue(tags);
  }

  protected submit(): void {
    if (this.personal.invalid) {
      this.personal.markAllAsTouched();
      return;
    }
    this.submitted.set(true);
    const name = this.personal.controls.name.value;
    this.snackBar.open(`Thanks ${name}! Your registration was submitted.`, 'Dismiss', {
      duration: 4000,
    });
  }

  protected restart(): void {
    this.submitted.set(false);
    this.stepper().reset();
    this.preferences.reset({ tags: [], framework: 'angular', newsletter: true, launchDate: null });
  }
}
