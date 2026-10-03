import { FocusMonitor } from '@angular/cdk/a11y';
import { BooleanInput, coerceBooleanProperty } from '@angular/cdk/coercion';
import {
  ChangeDetectionStrategy,
  Component,
  DoCheck,
  ElementRef,
  Input,
  OnDestroy,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { ControlValueAccessor, FormGroupDirective, NgControl, NgForm, Validators } from '@angular/forms';
import { ErrorStateMatcher } from '@angular/material/core';
import { MatFormFieldControl } from '@angular/material/form-field';
import { Subject } from 'rxjs';

let nextId = 0;

/**
 * A custom Material form-field control for a ternary "trit" string: only the digits 0, 1 and 2 are
 * accepted (Malbolge's native number system). Any other key presses or pasted characters are dropped.
 *
 * Implements both ControlValueAccessor (works with reactive forms) and MatFormFieldControl (works
 * inside <mat-form-field> with floating label, hint, error and required marker).
 */
@Component({
  selector: 'app-trit-input',
  // Eager: errorState/describedBy are plain fields updated outside of signals.
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <input
      #input
      class="trit-native"
      type="text"
      inputmode="numeric"
      autocomplete="off"
      spellcheck="false"
      [id]="id"
      [value]="value"
      [disabled]="disabled"
      [attr.maxlength]="maxLength"
      [attr.aria-describedby]="describedBy || null"
      [attr.aria-required]="required"
      [attr.aria-invalid]="errorState"
      [attr.placeholder]="shouldLabelFloat ? placeholder : null"
      (input)="onInput(input)"
      (blur)="onBlur()"
    />
    <span class="trit-count" aria-hidden="true">{{ value.length }}/{{ maxLength }}</span>
  `,
  styles: `
    :host {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .trit-native {
      flex: 1 1 auto;
      border: none;
      background: none;
      outline: none;
      padding: 0;
      margin: 0;
      font: inherit;
      color: inherit;
      letter-spacing: 0.3em;
      font-family: 'Cascadia Code', Consolas, Menlo, monospace;
      width: 100%;
    }
    .trit-count {
      font: var(--mat-sys-label-small);
      color: var(--mat-sys-on-surface-variant);
    }
  `,
  providers: [{ provide: MatFormFieldControl, useExisting: TritInput }],
  host: {
    '[class.floating]': 'shouldLabelFloat',
    '[id]': 'id + "-host"',
  },
})
export class TritInput implements ControlValueAccessor, MatFormFieldControl<string>, DoCheck, OnDestroy {
  private readonly focusMonitor = inject(FocusMonitor);
  private readonly elementRef = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly defaultMatcher = inject(ErrorStateMatcher);
  private readonly parentForm = inject(NgForm, { optional: true });
  private readonly parentFormGroup = inject(FormGroupDirective, { optional: true });
  readonly ngControl = inject(NgControl, { optional: true, self: true });

  private readonly inputRef = viewChild.required<ElementRef<HTMLInputElement>>('input');
  private readonly valueSig = signal('');

  readonly stateChanges = new Subject<void>();
  readonly id = `app-trit-input-${nextId++}`;
  readonly controlType = 'app-trit-input';
  focused = false;
  errorState = false;
  describedBy = '';

  @Input() maxLength = 9;

  private onChange: (value: string) => void = () => {};
  private onTouched: () => void = () => {};

  constructor() {
    if (this.ngControl) {
      this.ngControl.valueAccessor = this;
    }
    this.focusMonitor.monitor(this.elementRef, true).subscribe((origin) => {
      const focused = !!origin;
      if (this.focused !== focused) {
        this.focused = focused;
        this.stateChanges.next();
      }
    });
  }

  @Input()
  get value(): string {
    return this.valueSig();
  }
  set value(v: string | null) {
    this.valueSig.set(sanitize(v ?? '', this.maxLength));
    this.stateChanges.next();
  }

  @Input()
  get placeholder(): string {
    return this._placeholder;
  }
  set placeholder(v: string) {
    this._placeholder = v;
    this.stateChanges.next();
  }
  private _placeholder = '';

  @Input()
  get required(): boolean {
    return this._required ?? this.ngControl?.control?.hasValidator(Validators.required) ?? false;
  }
  set required(v: BooleanInput) {
    this._required = coerceBooleanProperty(v);
    this.stateChanges.next();
  }
  private _required: boolean | undefined;

  @Input()
  get disabled(): boolean {
    return this._disabled;
  }
  set disabled(v: BooleanInput) {
    this._disabled = coerceBooleanProperty(v);
    this.stateChanges.next();
  }
  private _disabled = false;

  get empty(): boolean {
    return this.valueSig().length === 0;
  }

  get shouldLabelFloat(): boolean {
    return this.focused || !this.empty;
  }

  ngDoCheck(): void {
    if (!this.ngControl) return;
    const parent = this.parentFormGroup ?? this.parentForm;
    const next = this.defaultMatcher.isErrorState(this.ngControl.control as never, parent);
    if (next !== this.errorState) {
      this.errorState = next;
      this.stateChanges.next();
    }
  }

  ngOnDestroy(): void {
    this.stateChanges.complete();
    this.focusMonitor.stopMonitoring(this.elementRef);
  }

  setDescribedByIds(ids: string[]): void {
    this.describedBy = ids.join(' ');
  }

  onContainerClick(): void {
    if (!this.disabled) {
      this.inputRef().nativeElement.focus();
    }
  }

  protected onInput(input: HTMLInputElement): void {
    const clean = sanitize(input.value, this.maxLength);
    if (clean !== input.value) {
      input.value = clean; // drop non-trit characters in place
    }
    this.valueSig.set(clean);
    this.onChange(clean);
    this.stateChanges.next();
  }

  protected onBlur(): void {
    this.onTouched();
    this.stateChanges.next();
  }

  // ControlValueAccessor
  writeValue(value: string | null): void {
    this.value = value;
  }
  registerOnChange(fn: (value: string) => void): void {
    this.onChange = fn;
  }
  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }
  setDisabledState(isDisabled: boolean): void {
    this.disabled = isDisabled;
  }

}

function sanitize(raw: string, max: number): string {
  return raw.replace(/[^012]/g, '').slice(0, max);
}
