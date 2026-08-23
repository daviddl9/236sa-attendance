import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { DobFields } from './dob-fields';

function setup(initial = '') {
  const onChange = vi.fn();
  function Harness() {
    const [value, setValue] = useState(initial);
    return (
      <DobFields
        value={value}
        onChange={(v) => {
          onChange(v);
          setValue(v);
        }}
      />
    );
  }
  render(<Harness />);
  return {
    onChange,
    day: screen.getByLabelText('day'),
    month: screen.getByLabelText('month'),
    year: screen.getByLabelText('year'),
  };
}

describe('DobFields', () => {
  it('renders three fields with the current value split', () => {
    const { day, month, year } = setup('15/03/1995');
    expect(day).toHaveValue('15');
    expect(month).toHaveValue('03');
    expect(year).toHaveValue('1995');
  });

  it('auto-advances to month when day is complete', async () => {
    const user = userEvent.setup();
    const { day, month, onChange } = setup();
    await user.type(day, '15');
    expect(month).toHaveFocus();
    expect(onChange).toHaveBeenLastCalledWith('15');
  });

  it('does not advance after a single digit day', async () => {
    const user = userEvent.setup();
    const { day, month } = setup();
    await user.type(day, '1');
    expect(day).toHaveFocus();
    expect(month).not.toHaveFocus();
  });

  it('auto-advances to year when month is complete', async () => {
    const user = userEvent.setup();
    const { month, year, onChange } = setup('15');
    await user.type(month, '03');
    expect(year).toHaveFocus();
    expect(onChange).toHaveBeenLastCalledWith('15/03');
  });

  it('emits the full date when all fields are typed', async () => {
    const user = userEvent.setup();
    const { day, month, year, onChange } = setup();
    await user.type(day, '15');
    await user.type(month, '03');
    await user.type(year, '1995');
    expect(onChange).toHaveBeenLastCalledWith('15/03/1995');
  });

  it('backspacing on an empty month returns focus to day', async () => {
    const user = userEvent.setup();
    const { day, month } = setup('15');
    month.focus();
    await user.keyboard('{Backspace}');
    expect(day).toHaveFocus();
  });

  it('backspacing on an empty year returns focus to month', async () => {
    const user = userEvent.setup();
    const { month, year } = setup('15/03');
    year.focus();
    await user.keyboard('{Backspace}');
    expect(month).toHaveFocus();
  });

  it('typing into a full day moves focus to month without changing day', async () => {
    const user = userEvent.setup();
    const { day, month, onChange } = setup('15/03/1995');
    await user.type(day, '2');
    expect(month).toHaveFocus();
    expect(day).toHaveValue('15');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('pastes a full date across fields', async () => {
    const user = userEvent.setup();
    const { day, month, year, onChange } = setup();
    await user.click(day);
    await user.paste('15031995');
    expect(day).toHaveValue('15');
    expect(month).toHaveValue('03');
    expect(year).toHaveValue('1995');
    expect(onChange).toHaveBeenLastCalledWith('15/03/1995');
  });

  it('strips non-digit input', async () => {
    const user = userEvent.setup();
    const { day, onChange } = setup();
    await user.type(day, '1a5');
    expect(day).toHaveValue('15');
    expect(onChange).toHaveBeenLastCalledWith('15');
  });
});
