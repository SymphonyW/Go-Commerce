import assert from 'node:assert/strict';
import { beforeEach, test } from 'node:test';
import api, { orderAPI } from './api.js';

beforeEach(() => {
  globalThis.localStorage = {
    getItem(key) {
      return key === 'token' ? 'test-token' : null;
    },
  };
});

test('createOrder sends an idempotency key header', async () => {
  let capturedConfig;
  api.defaults.adapter = async (config) => {
    capturedConfig = config;
    return {
      data: { order: { id: 1 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    };
  };

  await orderAPI.createOrder({ items: [{ product_id: 1, quantity: 1 }] });

  assert.equal(capturedConfig.url, '/orders');
  assert.equal(capturedConfig.method, 'post');
  assert.match(capturedConfig.headers.get('Idempotency-Key'), /^order-/);
});
