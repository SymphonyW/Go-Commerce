import assert from 'node:assert/strict';
import { test } from 'node:test';
import { centsToInputValue, formatMoney, parseMoneyToCents } from './display.js';

const moneyFormatter = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
});

test('formatMoney formats integer cents', () => {
  assert.equal(formatMoney(0), moneyFormatter.format(0));
  assert.equal(formatMoney(1), moneyFormatter.format(0.01));
  assert.equal(formatMoney(129900), moneyFormatter.format(1299));
});

test('parseMoneyToCents parses decimal input without float arithmetic', () => {
  assert.equal(parseMoneyToCents('0'), 0);
  assert.equal(parseMoneyToCents('0.01'), 1);
  assert.equal(parseMoneyToCents('1299.00'), 129900);
  assert.equal(parseMoneyToCents('10.999'), null);
  assert.equal(parseMoneyToCents('-1'), null);
  assert.equal(parseMoneyToCents('900719925474099.99'), null);
});

test('centsToInputValue converts cents for form editing', () => {
  assert.equal(centsToInputValue(0), '0');
  assert.equal(centsToInputValue(1), '0.01');
  assert.equal(centsToInputValue(129900), '1299');
});
