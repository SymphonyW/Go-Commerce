import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import MerchantConsoleNav from '../components/MerchantConsoleNav';
import { merchantAPI } from '../services/api';

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
      } catch (err) {
        setError(err.response?.data?.error || '获取商家订单失败');
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

      <section className="merchant-section">
        <div className="section-heading">
          <div>
            <p className="eyebrow">订单管理</p>
            <h1>相关订单</h1>
            <p>这里只展示与当前店铺商品有关的订单项。</p>
          </div>
        </div>

        {error && <div className="error-message">{error}</div>}

        {loading ? (
          <div className="loading">加载中...</div>
        ) : orders.length === 0 ? (
          <div className="empty-state compact">暂无相关订单</div>
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
                  <span className={`status-pill ${order.status}`}>{order.status}</span>
                  <span>
                    {(order.items || []).map((item) => `${item.product_name} × ${item.quantity}`).join('，')}
                  </span>
                  <span>¥{Number(order.total_amount || 0).toFixed(2)}</span>
                  <span>{new Date(order.created_at).toLocaleString()}</span>
                </div>
              ))}
            </div>

            <div className="pagination">
              <button className="btn btn-secondary" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>
                上一页
              </button>
              <span>
                第 {page} / {totalPages} 页
              </span>
              <button className="btn btn-secondary" disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>
                下一页
              </button>
            </div>
          </>
        )}
      </section>
    </div>
  );
};

export default MerchantOrders;
