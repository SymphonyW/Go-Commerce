import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import MerchantConsoleNav from '../components/MerchantConsoleNav';
import PageHeader from '../components/PageHeader';
import SectionCard from '../components/SectionCard';
import StatusBadge from '../components/StatusBadge';
import { merchantAPI } from '../services/api';
import { formatMoney, formatDateTime, getOrderStatusLabel } from '../utils/display';

const MerchantOrders = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const merchantId = searchParams.get('merchant_id') || undefined;
  const [orders, setOrders] = useState([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const pageSize = 8;

  useEffect(() => {
    const token = localStorage.getItem('token');
    const role = localStorage.getItem('role');
    if (!token) {
      navigate('/login');
      return;
    }
    if (role !== 'merchant' && role !== 'admin') {
      navigate('/');
    }
  }, [navigate]);

  useEffect(() => {
    const loadOrders = async () => {
      try {
        setLoading(true);
        setError('');
        const data = await merchantAPI.listConsoleOrders({
          ...(merchantId ? { merchant_id: merchantId } : {}),
          page,
          page_size: pageSize,
        });
        setOrders(data.orders || []);
        setTotal(data.total || 0);
      } catch (loadError) {
        setError(loadError.response?.data?.error || '获取商家订单失败');
      } finally {
        setLoading(false);
      }
    };

    loadOrders();
  }, [merchantId, page]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="merchant-console">
      <MerchantConsoleNav />

      <PageHeader
        eyebrow="订单管理"
        title="相关订单"
        subtitle="这里只展示与当前店铺商品有关的订单项。"
        meta={`${total} 笔`}
      />

      {error && <div className="error-message">{error}</div>}

      <SectionCard>
        {loading ? (
          <LoadingState label="正在加载订单..." />
        ) : orders.length === 0 ? (
          <EmptyState compact title="暂无相关订单" description="一旦有商品成交，这里会沉淀店铺订单记录。" />
        ) : (
          <>
            <div className="merchant-table order-table">
              <div className="merchant-table-head">
                <span>订单号</span>
                <span>状态</span>
                <span>商品</span>
                <span>金额</span>
                <span>下单时间</span>
              </div>
              {orders.map((order) => (
                <div key={order.id} className="merchant-table-row">
                  <span>#{order.id}</span>
                  <StatusBadge status={order.status} label={getOrderStatusLabel(order.status, order.cancel_reason)} />
                  <span>{(order.items || []).map((item) => `${item.product_name} × ${item.quantity}`).join('，')}</span>
                  <span>{formatMoney(order.total_amount_cents)}</span>
                  <span>{formatDateTime(order.created_at)}</span>
                </div>
              ))}
            </div>

            <div className="pagination">
              <button type="button" className="btn btn-secondary btn-sm" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>
                上一页
              </button>
              <span>
                第 {page} / {totalPages} 页
              </span>
              <button type="button" className="btn btn-secondary btn-sm" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>
                下一页
              </button>
            </div>
          </>
        )}
      </SectionCard>
    </div>
  );
};

export default MerchantOrders;
