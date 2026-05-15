// 订单详情页面
// 显示订单的详细信息，包括订单状态、商品列表和总金额
// 提供取消订单功能
import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { orderAPI, paymentAPI } from '../services/api';

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
  // 当前支付记录
  const [payment, setPayment] = useState(null);
  // 支付动作加载状态
  const [paying, setPaying] = useState(false);
  // 确认收货动作加载状态
  const [completing, setCompleting] = useState(false);

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

  const refreshOrder = async () => {
    const updatedOrder = await orderAPI.getOrder(id);
    setOrder(updatedOrder.order);
  };

  // 创建模拟支付记录，后续由用户手动触发成功或失败，便于演示完整状态流转。
  const handleCreatePayment = async () => {
    try {
      setPaying(true);
      const response = await paymentAPI.createPayment({
        order_id: Number(id),
        payment_method: 'mock_balance',
      });
      setPayment(response.payment);
    } catch (error) {
      console.error('创建支付失败:', error);
      alert(error.response?.data?.error || '创建支付失败');
    } finally {
      setPaying(false);
    }
  };

  const handlePaymentSuccess = async () => {
    if (!payment) {
      return;
    }
    try {
      setPaying(true);
      const response = await paymentAPI.markSuccess(payment.id);
      setPayment(response.payment);
      await refreshOrder();
      alert('支付成功');
    } catch (error) {
      console.error('模拟支付成功失败:', error);
      alert(error.response?.data?.error || '支付失败');
    } finally {
      setPaying(false);
    }
  };

  const handlePaymentFail = async () => {
    if (!payment) {
      return;
    }
    try {
      setPaying(true);
      const response = await paymentAPI.markFailed(payment.id);
      setPayment(response.payment);
      alert('已模拟支付失败，订单仍保持待支付');
    } catch (error) {
      console.error('模拟支付失败操作异常:', error);
      alert(error.response?.data?.error || '支付失败操作异常');
    } finally {
      setPaying(false);
    }
  };

  const handleCompleteOrder = async () => {
    if (!window.confirm('确认已经收到商品了吗？')) {
      return;
    }
    try {
      setCompleting(true);
      await orderAPI.completeOrder(id);
      await refreshOrder();
      alert('已确认收货');
    } catch (error) {
      console.error('确认收货失败:', error);
      alert(error.response?.data?.error || '确认收货失败');
    } finally {
      setCompleting(false);
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
              {order.status === 'pending' ? '待支付' :
               order.status === 'paid' ? '已支付' :
               order.status === 'shipped' ? '已发货' :
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
            {order.status === 'pending' && !payment && (
              <button
                onClick={handleCreatePayment}
                disabled={paying}
                className="btn btn-sm"
              >
                {paying ? '创建支付中...' : '去支付'}
              </button>
            )}
            {order.status === 'shipped' && (
              <button
                onClick={handleCompleteOrder}
                disabled={completing}
                className="btn btn-sm"
              >
                {completing ? '确认中...' : '确认收货'}
              </button>
            )}
          </div>
        </div>
        
        {/* 订单基本信息 */}
        <div className="order-detail-info">
          <div className="order-detail-date">下单时间: {order.created_at}</div>
          <div className="order-detail-total">总金额: ¥{order.total_amount}</div>
        </div>

        {payment && order.status === 'pending' && (
          <div className="payment-actions">
            <h3>支付模拟</h3>
            <div>支付单号: {payment.payment_no}</div>
            <div>支付状态: {payment.status}</div>
            {payment.status === 'created' && (
              <div className="payment-buttons">
                <button onClick={handlePaymentSuccess} disabled={paying} className="btn btn-sm">
                  模拟支付成功
                </button>
                <button onClick={handlePaymentFail} disabled={paying} className="btn btn-danger btn-sm">
                  模拟支付失败
                </button>
              </div>
            )}
            {payment.status === 'failed' && (
              <div className="payment-buttons">
                <button onClick={handleCreatePayment} disabled={paying} className="btn btn-sm">
                  {paying ? '重新创建中...' : '重新发起支付'}
                </button>
              </div>
            )}
          </div>
        )}

        {order.status === 'paid' && (
          <div className="payment-actions">订单已支付，等待商家发货</div>
        )}

        {order.status === 'completed' && (
          <div className="payment-actions">订单已完成</div>
        )}
        
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
