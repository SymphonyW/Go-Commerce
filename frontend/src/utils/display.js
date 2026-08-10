export const CATEGORY_OPTIONS = ['数码科技', '居家生活', '户外骑行', '图书学习'];

export const getProductImageUrl = (url, size = 'card') => {
  if (!url) {
    return 'https://loremflickr.com/640/420/product?lock=0';
  }

  if (url.includes('wikipedia.org/wiki/File:')) {
    const fileName = url.split('/').pop();
    const width = size === 'detail' ? '640px' : '320px';
    return `https://upload.wikimedia.org/wikipedia/commons/thumb/${fileName.charAt(0)}/${fileName.charAt(0) + fileName.charAt(1)}/${fileName}/${width}-${fileName}`;
  }

  return url;
};

const moneyFormatter = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
});

export const formatMoney = (cents) => {
  const value = Number(cents);
  return moneyFormatter.format(Number.isFinite(value) ? value / 100 : 0);
};

export const parseMoneyToCents = (value) => {
  const normalized = String(value ?? '').trim().replace(/,/g, '');
  if (!/^\d+(?:\.\d{0,2})?$/.test(normalized)) {
    return null;
  }

  const [whole, fraction = ''] = normalized.split('.');
  const cents = Number.parseInt(whole, 10) * 100 + Number.parseInt(`${fraction}00`.slice(0, 2), 10);
  return Number.isSafeInteger(cents) ? cents : null;
};

export const centsToInputValue = (cents) => {
  const value = Number(cents);
  if (!Number.isSafeInteger(value) || value < 0) {
    return '';
  }

  const whole = Math.trunc(value / 100);
  const fraction = value % 100;
  return fraction === 0 ? String(whole) : `${whole}.${String(fraction).padStart(2, '0')}`;
};


export const formatDateTime = (value) => {
  if (!value) return '--';
  return new Date(value).toLocaleString('zh-CN', {
    hour12: false,
  });
};

export const getOrderStatusLabel = (status, cancelReason) => {
  if (status === 'pending') return '待支付';
  if (status === 'paid') return '已支付';
  if (status === 'shipped') return '已发货';
  if (status === 'completed') return '已完成';
  if (status === 'cancelled' && cancelReason === 'payment_timeout') return '已取消（支付超时）';
  if (status === 'cancelled') return '已取消';
  return status || '未知状态';
};

export const getStockMeta = (stock) => {
  if (stock <= 0) return { label: '暂时缺货', tone: 'danger' };
  if (stock <= 12) return { label: `低库存 · ${stock}`, tone: 'warning' };
  return { label: `现货 · ${stock}`, tone: 'success' };
};
