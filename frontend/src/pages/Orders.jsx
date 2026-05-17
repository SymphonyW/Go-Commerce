import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import PageHeader from '../components/PageHeader';
import StatusBadge from '../components/StatusBadge';
import { orderAPI } from '../services/api';
import { formatCurrency, formatDateTime, getOrderStatusLabel } from '../utils/display';

const Orders = () => {
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    const fetchOrders = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        navigate('/login');
        return;
      }

      try {
        const data = await orderAPI.listOrders();
        setOrders(data.orders || []);
      } catch (fetchError) {
        console.error('Failed to fetch orders:', fetchError);
        setError('获取订单列表失败');
      } finally {
        setLoading(false);
      }
    };

    fetchOrders();
  }, [navigate]);

  if (loading) {
    return <LoadingState label="正在加载订单..." />;
  }

  return (
    <div className="orders-container">
      <PageHeader
        eyebrow="Orders"
        title="我的订单"
        subtitle="查看订单状态、金额与下单时间。"
        meta={`${orders.length} 笔`}
      />

      {error && <div className="error-message">{error}</div>}

      {orders.length === 0 ? (
        <EmptyState
          title="还没有订单"
          description="完成一次购买后，这里会沉淀完整的交易记录。"
          icon="⌁"
          action={
            <Link to="/products" className="btn btn-primary">
              去购物
            </Link>
          }
        />
      ) : (
        <div className="orders-list">
          {orders.map((order) => (
            <article key={order.id} className="order-card">
              <div className="order-card-header">
                <div>
                  <p>订单号</p>
                  <h3>#{order.id}</h3>
                </div>
                <StatusBadge status={order.status} label={getOrderStatusLabel(order.status, order.cancel_reason)} />
              </div>
              <div className="order-card-meta">
                <div>
                  <p>总金额</p>
                  <strong>{formatCurrency(order.total_amount)}</strong>
                </div>
                <div>
                  <p>下单时间</p>
                  <strong>{formatDateTime(order.created_at)}</strong>
                </div>
              </div>
              <div className="order-card-footer">
                <span>交易记录已按当前状态归档</span>
                <Link to={`/orders/${order.id}`} className="btn btn-secondary btn-sm">
                  查看详情
                </Link>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
};

export default Orders;
