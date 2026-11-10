import { describe, expect, it } from 'vitest';
import { readBudget } from './budget';

describe('readBudget', () => {
  it('finds no budget without the annotation', () => {
    expect(readBudget(undefined, 100)).toBeNull();
  });

  it.each([
    ['500', 500],
    [' 500 ', 500],
    ['500.5', 500.5],
    ['1250.75', 1250.75],
    ['0.01', 0.01],
  ])('reads %j as an amount', (annotation, amount) => {
    expect(readBudget(annotation, 1)).toMatchObject({ kind: 'set', amount });
  });

  it.each(['', '   ', '500 USD', '$500', 'abc', '0', '0.00', '-5', '+5', '1,000', '1e3', '0x1F4', 'Infinity', '500.', '.5', '500.125'])(
    'cannot read %j',
    annotation => {
      expect(readBudget(annotation, 1)).toEqual({ kind: 'unreadable', value: annotation.trim() });
    }
  );

  it('cannot read a number too long to hold', () => {
    expect(readBudget('9'.repeat(400), 1)?.kind).toBe('unreadable');
  });

  it('shortens a long value it cannot read', () => {
    expect(readBudget('x'.repeat(5000), 1)).toEqual({ kind: 'unreadable', value: `${'x'.repeat(60)}…` });
  });

  it.each([
    [0, 'within', '0%'],
    [340.18, 'within', '68%'],
    [375, 'within', '75%'],
    [375.05, 'close', '75.0%'],
    [377, 'close', '75.4%'],
    [380, 'close', '76%'],
    [450, 'close', '90%'],
    [452, 'nearly', '90.4%'],
    [472.4, 'nearly', '94%'],
    [500, 'nearly', '100%'],
    [500.01, 'over', '100.0%'],
    [501.5, 'over', '100.3%'],
    [590.75, 'over', '118%'],
  ])('at $%d of $500 is %s, shown as %s', (cost, level, share) => {
    expect(readBudget('500', cost)).toMatchObject({ level, share });
  });

  // These divide to a hair above 90 in floating point.
  it.each([
    ['289', 260.1, 'close', '90%'],
    ['573', 515.7, 'close', '90%'],
    ['1141', 1026.9, 'close', '90%'],
    ['289', 260.11, 'nearly', '90.0%'],
  ])('at a budget of $%s, $%d is %s, shown as %s', (annotation, cost, level, share) => {
    expect(readBudget(annotation, cost)).toMatchObject({ level, share });
  });
});
