import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { orderAPI } from '../services/api';

const OrderDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [order, setOrder] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchOrder = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        navigate('/login');
        return;
      }

      try {
        const data = await orderAPI.getOrder(id);
        setOrder(data.order);
      } catch (error) {
        console.error('Failed to fetch order:', error);
        setError('获取订单详情失败');
      } finally {
        setLoading(false);
      }
    };

    fetchOrder();
  }, [id, navigate]);

  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  if (error || !order) {
    return <div className="error-message">{error || '订单不存在'}</div>;
  }

  return (
    <div className="order-detail">
      <h1>订单详情</h1>
      <div className="order-detail-card">
        <div className="order-detail-header">
          <div className="order-detail-id">订单号: {order.id}</div>
          <div className={`order-detail-status ${order.status}`}>
            {order.status === 'pending' ? '待处理' : 
             order.status === 'completed' ? '已完成' : 
             order.status === 'cancelled' ? '已取消' : order.status}
          </div>
        </div>
        <div className="order-detail-info">
          <div className="order-detail-date">下单时间: {order.created_at}</div>
          <div className="order-detail-total">总金额: ¥{order.total_amount}</div>
        </div>
        <div className="order-detail-items">
          <h3>订单商品</h3>
          <div className="order-items-list">
            {order.items.map((item, index) => (
              <div key={index} className="order-item">
                <div className="order-item-name">{item.product_name}</div>
                <div className="order-item-quantity">数量: {item.quantity}</div>
                <div className="order-item-price">单价: ¥{item.price}</div>
                <div className="order-item-subtotal">小计: ¥{(item.price * item.quantity).toFixed(2)}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

export default OrderDetail;