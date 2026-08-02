import assert from 'node:assert/strict';
import { beforeEach, test } from 'node:test';
import api, { getAPIErrorMessage, orderAPI, paymentAPI } from './api.js';

beforeEach(() => {
  globalThis.localStorage = {
    getItem(key) {
      return key === 'token' ? 'test-token' : null;
    },
  };
});

test('auth token is injected into requests', async () => {
  let capturedConfig;
  api.defaults.adapter = async (config) => {
    capturedConfig = config;
    return {
      data: { orders: [] },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    };
  };

  await orderAPI.listOrders();

  assert.equal(capturedConfig.url, '/orders');
  assert.equal(capturedConfig.headers.get('Authorization'), 'Bearer test-token');
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

test('markSuccess sends an idempotency key header', async () => {
  let capturedConfig;
  api.defaults.adapter = async (config) => {
    capturedConfig = config;
    return {
      data: { payment: { id: 10 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    };
  };

  await paymentAPI.markSuccess(10);

  assert.equal(capturedConfig.url, '/payments/10/success');
  assert.equal(capturedConfig.method, 'post');
  assert.match(capturedConfig.headers.get('Idempotency-Key'), /^payment-/);
});

test('cancelOrder sends an idempotency key header', async () => {
  let capturedConfig;
  api.defaults.adapter = async (config) => {
    capturedConfig = config;
    return {
      data: { success: true },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    };
  };

  await orderAPI.cancelOrder(4);

  assert.equal(capturedConfig.url, '/orders/4/cancel');
  assert.equal(capturedConfig.method, 'put');
  assert.match(capturedConfig.headers.get('Idempotency-Key'), /^order-cancel-/);
});

test('getAPIErrorMessage prefers backend error payloads', () => {
  assert.equal(
    getAPIErrorMessage(
      {
        response: {
          data: {
            code: 'ORDER_NOT_PAYABLE',
            message: 'order is not payable',
            error: 'legacy error',
            request_id: 'req-1',
          },
        },
      },
      'fallback',
    ),
    'order is not payable',
  );
  assert.equal(
    getAPIErrorMessage({ response: { data: { error: 'insufficient stock' } } }, 'fallback'),
    'insufficient stock',
  );
  assert.equal(
    getAPIErrorMessage({ response: { data: { message: 'order not payable' } } }, 'fallback'),
    'order not payable',
  );
  assert.equal(getAPIErrorMessage(new Error('network'), 'fallback'), 'fallback');
});

test('api base URL is read from the environment', async () => {
  process.env.VITE_API_BASE_URL = 'http://example.test/api';
  try {
    const moduleURL = new URL('./api.js', import.meta.url);
    moduleURL.searchParams.set('cache_bust', Date.now().toString());
    const { default: isolatedAPI } = await import(moduleURL.href);

    assert.equal(isolatedAPI.defaults.baseURL, 'http://example.test/api');
  } finally {
    delete process.env.VITE_API_BASE_URL;
  }
});
