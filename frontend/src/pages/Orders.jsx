import { useEffect, useState } from 'react';
import { orderAPI } from '../services/api';
import { Link, useNavigate } from 'react-router-dom';

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
      } catch (error) {
        console.error('Failed to fetch orders:', error);
        setError('获取订单列表失败');
      } finally {
        setLoading(false);
      }
    };

    fetchOrders();
  }, [navigate]);

  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="orders-container">
      <h1>我的订单</h1>
      {error && <div className="error-message">{error}</div>}
      {orders.length === 0 ? (
        <div className="empty-state">
          <p>您还没有订单</p>
          <Link to="/products" className="btn">
            去购物
          </Link>
        </div>
      ) : (
        <div className="orders-list">
          {orders.map((order) => (
            <div key={order.id} className="order-card">
              <div className="order-header">
                <div className="order-id">订单号: {order.id}</div>
                <div className={`order-status ${order.status}`}>
                  {order.status === 'pending' ? '待处理' : 
                   order.status === 'completed' ? '已完成' : 
                   order.status === 'cancelled' ? '已取消' : order.status}
                </div>
              </div>
              <div className="order-info">
                <div className="order-total">总金额: ¥{order.total_amount}</div>
                <div className="order-date">下单时间: {order.created_at}</div>
              </div>
              <Link to={`/orders/${order.id}`} className="btn btn-sm">
                查看详情
              </Link>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default Orders;