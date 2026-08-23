import { describe, expect, it } from 'vitest';
import { isValidDob, joinDob, splitDob } from './dob-format';

describe('splitDob', () => {
  it('splits dd/mm/yyyy into parts', () => {
    expect(splitDob('15/03/1995')).toEqual({ day: '15', month: '03', year: '1995' });
  });

  it('handles partial and empty values', () => {
    expect(splitDob('15/03')).toEqual({ day: '15', month: '03', year: '' });
    expect(splitDob('')).toEqual({ day: '', month: '', year: '' });
  });

  it('ignores non-digits', () => {
    expect(splitDob('15x03y1995')).toEqual({ day: '15', month: '03', year: '1995' });
  });
});

describe('joinDob', () => {
  it('joins parts into dd/mm/yyyy', () => {
    expect(joinDob('15', '03', '1995')).toBe('15/03/1995');
  });

  it('omits empty trailing parts', () => {
    expect(joinDob('15', '03', '')).toBe('15/03');
    expect(joinDob('15', '', '')).toBe('15');
  });
});

describe('isValidDob', () => {
  it('accepts a valid date', () => {
    expect(isValidDob('15/03/1995')).toBe(true);
  });

  it('accepts compact ddmmyyyy', () => {
    expect(isValidDob('15031995')).toBe(true);
  });

  it('rejects out-of-range day', () => {
    expect(isValidDob('32/03/1995')).toBe(false);
    expect(isValidDob('00/03/1995')).toBe(false);
  });

  it('rejects out-of-range month', () => {
    expect(isValidDob('15/13/1995')).toBe(false);
    expect(isValidDob('15/00/1995')).toBe(false);
  });

  it('rejects out-of-range year', () => {
    expect(isValidDob('15/03/1899')).toBe(false);
    expect(isValidDob('15/03/2999')).toBe(false);
  });

  it('rejects incomplete values', () => {
    expect(isValidDob('15/03')).toBe(false);
    expect(isValidDob('')).toBe(false);
  });
});
