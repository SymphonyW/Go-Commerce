// 订单详情页面
// 显示订单的详细信息，包括订单状态、商品列表和总金额
// 提供取消订单功能
import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { orderAPI } from '../services/api';

const OrderDetail = () => {
  // 获取URL参数中的订单ID
  const { id } = useParams();
  // 导航对象，用于跳转到其他页面
  const navigate = useNavigate();
  // 订单数据状态
  const [order, setOrder] = useState(null);
  // 加载状态
  const [loading, setLoading] = useState(true);
  // 错误信息状态
  const [error, setError] = useState('');
  // 取消订单的加载状态
  const [cancelling, setCancelling] = useState(false);

  // 组件挂载时获取订单详情
  useEffect(() => {
    const fetchOrder = async () => {
      // 检查是否有登录令牌
      const token = localStorage.getItem('token');
      if (!token) {
        // 未登录，跳转到登录页面
        navigate('/login');
        return;
      }

      try {
        // 调用API获取订单详情
        const data = await orderAPI.getOrder(id);
        setOrder(data.order);
      } catch (error) {
        console.error('Failed to fetch order:', error);
        setError('获取订单详情失败');
      } finally {
        // 无论成功失败，都设置加载状态为false
        setLoading(false);
      }
    };

    fetchOrder();
  }, [id, navigate]);

  // 加载中状态
  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  // 取消订单处理函数
  const handleCancelOrder = async () => {
    // 确认用户是否要取消订单
    if (window.confirm('确定要取消这个订单吗？')) {
      try {
        // 设置取消中状态
        setCancelling(true);
        // 调用API取消订单
        const response = await orderAPI.cancelOrder(id);
        if (response.success) {
          // 取消成功，重新获取订单信息
          const updatedOrder = await orderAPI.getOrder(id);
          setOrder(updatedOrder.order);
          alert('订单取消成功');
        } else {
          // 取消失败，显示错误信息
          alert(response.message);
        }
      } catch (error) {
        console.error('取消订单失败:', error);
        alert('取消订单失败，请稍后重试');
      } finally {
        // 无论成功失败，都设置取消中状态为false
        setCancelling(false);
      }
    }
  };

  // 错误状态或订单不存在
  if (error || !order) {
    return <div className="error-message">{error || '订单不存在'}</div>;
  }

  // 渲染订单详情
  return (
    <div className="order-detail">
      <h1>订单详情</h1>
      <div className="order-detail-card">
        {/* 订单头部信息 */}
        <div className="order-detail-header">
          <div className="order-detail-id">订单号: {order.id}</div>
          <div className="order-detail-status-container">
            {/* 订单状态 */}
            <div className={`order-detail-status ${order.status}`}>
              {order.status === 'pending' ? '待处理' : 
               order.status === 'completed' ? '已完成' : 
               order.status === 'cancelled' ? '已取消' : order.status}
            </div>
            {/* 取消订单按钮（仅当订单状态为待处理时显示） */}
            {order.status === 'pending' && (
              <button 
                onClick={handleCancelOrder}
                disabled={cancelling}
                className="btn btn-danger btn-sm"
              >
                {cancelling ? '取消中...' : '取消订单'}
              </button>
            )}
          </div>
        </div>
        
        {/* 订单基本信息 */}
        <div className="order-detail-info">
          <div className="order-detail-date">下单时间: {order.created_at}</div>
          <div className="order-detail-total">总金额: ¥{order.total_amount}</div>
        </div>
        
        {/* 订单商品列表 */}
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