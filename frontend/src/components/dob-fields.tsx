import * as React from 'react';
import { joinDob, splitDob } from '../lib/dob-format';
import { Input } from './ui/input';

type Field = 'day' | 'month' | 'year';

const FIELDS: Record<
  Field,
  { max: number; next: Field | null; prev: Field | null; placeholder: string; autoComplete: string }
> = {
  day: { max: 2, next: 'month', prev: null, placeholder: 'DD', autoComplete: 'bday-day' },
  month: { max: 2, next: 'year', prev: 'day', placeholder: 'MM', autoComplete: 'bday-month' },
  year: { max: 4, next: null, prev: 'month', placeholder: 'YYYY', autoComplete: 'bday-year' },
};

interface DobFieldsProps {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  /** Applied to the day input so a Label can target the group. */
  id?: string;
}

export function DobFields({ value, onChange, disabled, id }: DobFieldsProps) {
  const dayRef = React.useRef<HTMLInputElement>(null);
  const monthRef = React.useRef<HTMLInputElement>(null);
  const yearRef = React.useRef<HTMLInputElement>(null);
  const refs = { day: dayRef, month: monthRef, year: yearRef };

  const parts = splitDob(value);

  const handleChange = (field: Field) => (e: React.ChangeEvent<HTMLInputElement>) => {
    const rawDigits = e.target.value.replace(/\D/g, '');
    const max = FIELDS[field].max;
    const next = { ...parts };
    next[field] = rawDigits.slice(0, max);

    // Paste (or overflow): distribute remaining digits across following fields.
    let rest = rawDigits.slice(max);
    let lastFilled: Field = field;
    let cursor: Field | null = FIELDS[field].next;
    while (cursor && rest) {
      next[cursor] = rest.slice(0, FIELDS[cursor].max);
      rest = rest.slice(FIELDS[cursor].max);
      lastFilled = cursor;
      cursor = FIELDS[cursor].next;
    }

    onChange(joinDob(next.day, next.month, next.year));

    if (rawDigits.length > max) {
      const afterLast = FIELDS[lastFilled].next;
      (afterLast ? refs[afterLast] : refs[lastFilled]).current?.focus();
    } else if (next[field].length === max) {
      const nextField = FIELDS[field].next;
      if (nextField) refs[nextField].current?.focus();
    }
  };

  const handleKeyDown = (field: Field) => (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (parts[field] === '') {
      if (e.key === 'Backspace') {
        e.preventDefault();
        const prev = FIELDS[field].prev;
        if (prev) refs[prev].current?.focus();
      }
      return;
    }
    // Field is full: swallow the digit and move focus so the next key lands there.
    if (
      parts[field].length === FIELDS[field].max &&
      /^[0-9]$/.test(e.key) &&
      !e.ctrlKey &&
      !e.metaKey &&
      !e.altKey
    ) {
      e.preventDefault();
      const next = FIELDS[field].next;
      if (next) refs[next].current?.focus();
    }
  };

  return (
    <div className="flex items-center gap-1.5">
      {(Object.keys(FIELDS) as Field[]).map((field, i) => (
        <React.Fragment key={field}>
          {i > 0 && <span className="text-muted-foreground">/</span>}
          <Input
            ref={refs[field]}
            id={field === 'day' ? id : undefined}
            type="text"
            inputMode="numeric"
            placeholder={FIELDS[field].placeholder}
            autoComplete={FIELDS[field].autoComplete}
            aria-label={field}
            className="w-16 text-center"
            value={parts[field]}
            onChange={handleChange(field)}
            onKeyDown={handleKeyDown(field)}
            disabled={disabled}
            required
          />
        </React.Fragment>
      ))}
    </div>
  );
}
